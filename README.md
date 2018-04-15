## Overview

Scale-provisioner is one of the dynamic provisioner which uses IBM Spectrum Scale as a storage backend for POD volumes in kubernetes clusters. Scale-provisioner dyanmically creates a persistent voluem as a fileset of Spectrum Scale on its file system.

## Building and Running Scale Dynamic Provisioner

In order to build scale-provisioner, you must

- have IBM Spectrum Scale cluster configured
- have go version 1.7 or greater installed
- have Docker installed


Our scale-provisioner Go package has many dependencies so it's a good idea to use a kubernetes-incubator/external-storage.

`go get` kubernetes-incubator/external-storage

```console
$ go get github.com/kubernetes-incubator/external-storage/ 
```

`clone` scale-provisioner under the extenal-storage directory.

```console
$ cd $GOPATH/src/github.com/kubernetes-incubator/external-storage/
$ git clone https://github.com/kohheinomura/scale-provisioner.git
$ cd scale-provisioner
```

Run make to build docker image nameed `scale-provisioner`. Note that the Docker image needs to be on the node we'll run the pod on. So you may need to tag your image and push it to Docker Hub so that it can be pulled later by the node, or just work on the node and build the image there. 

```console
$ make
```

Now we can specify our image in a pod. Since we are running our provisioner in a container as a pod, we should mount a corresponding `hostpath`volumes there to serve as the member of spectrum scale cluster.

Recall that you need to specify correct `PV_DIR` and `FS_NAME` corresponding to your environment.


```console
$ cat > scale-provisioner.yaml << EOF
kind: Pod
apiVersion: v1
metadata:
  name: scale-provisioner
spec:
  serviceAccount: scale-provisioner-account
  hostNetwork: true
  hostPID: true
  hostIPC: true
  containers:
    - name: scale-provisioner
      image: scale-provisioner:latest
      securityContext:
        privileged: true
      imagePullPolicy: "IfNotPresent"
      env:
        - name: PV_DIR
          value: "/gpfs/fs1"
        - name: FS_NAME
          value: "fs1"
      volumeMounts:
        - name: fs
          mountPath: /gpfs/fs1/
        - name: mmfs
          mountPath: /usr/lpp/mmfs/
        - name: var-mmfs
          mountPath: /var/mmfs/
        - name: var-ras
          mountPath: /var/adm/ras/
        - name: usr-lib-x86-64-linux-gnu
          mountPath: /usr/lib/x86_64-linux-gnu/
        - name: lib-x86-64-linux-gnu
          mountPath: /lib/x86_64-linux-gnu/
        - name: usr-bin
          mountPath: /usr/bin
        - name: run
          mountPath: /run
  nodeSelector:
    beta.kubernetes.io/arch: amd64
  volumes:
    - name: fs
      hostPath:
        path: /gpfs/fs1/
    - name: mmfs
      hostPath:
        path: /usr/lpp/mmfs/
    - name: var-mmfs
      hostPath:
        path: /var/mmfs/
    - name: var-ras
      hostPath:
        path: /var/adm/ras/
    - name: usr-lib-x86-64-linux-gnu
      hostPath:
        path: /usr/lib/x86_64-linux-gnu/
    - name: lib-x86-64-linux-gnu
      hostPath:
        path: /lib/x86_64-linux-gnu/
    - name: usr-bin
      hostPath:
        path: /usr/bin
    - name: run
      hostPath:
        path: /run
EOF
```

## Authorizing provisioners for RBAC

Our scale-provisioner runs on a cluster with RBAC enabled so you must create the following service account, role, and binding.

```console
$ cat > scale-account.yaml <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: scale-provisioner-account
EOF
$ kubectl create -f scale-account.yaml
```

```console
$ cat > scale-role.yaml <<EOF
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: scale-provisioner-role
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
EOF
$ kubectl create -f scale-role.yaml
```

```console
$ cat > scale-binding.yaml <<EOF
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: scale-provisioner-binding
subjects:
  - kind: ServiceAccount
    name: scale-provisioner-account
    namespace: default
roleRef:
  kind: ClusterRole
  name: scale-provisioner-role
  apiGroup: rbac.authorization.k8s.io
EOF
$ kubectl create -f scale-binding.yaml
```

## Using our Spectrum Scale Dynamic Provisioner

Deploy scale-provisioer.

```console
$ kubectl create -f scale-provisioner.yaml
```

Before proceeding, we check that it doesn't immediately crash due to one of the fatal conditions we wrote.

```console
$ kubectl get pods
NAME                                      READY     STATUS    RESTARTS   AGE
scale-provisioner                         1/1       Running   0          5s 
```

Now we create a StorageClass & PersistentVolumeClaim and see that a PersistentVolume is automatically created on the spectrum scale file system.


```console
$ cat > scale-storage-class.yaml << EOF
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: spectrum-scale
provisioner: ccc.ibm.co.jp/scale
EOF
$ kubectl create -f scale-storage-class.yaml
```

The following yaml file specifies the details of volume and we requested the persistent volume with size of 2G from spectrum scale storage class.

```console
$ cat > test-claim.yaml << EOF
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: my-scale-claim
  annotations:
    volume.beta.kubernetes.io/storage-class: "spectrum-scale"
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 2G
EOF
$ kubectl create -f test-claim.yaml
```
Review the status of the new persistent volume claim. You can see that the persistent volume claim ‘`my-scale-claim` is bound to the persistent volume ‘pvc-ff3856b0-3eeb-11e8-a874-0050569b54a8’.

```console
$ kubectl get pvc
NAME             STATUS    VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS          AGE
my-scale-claim   Bound     pvc-ff3856b0-3eeb-11e8-a874-0050569b54a8   2G         RWX            spectrum-scale        4s
```

```console
$ kubectl get pv
NAME                                       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS    CLAIM                    STORAGECLASS          REASON    AGE
pvc-ff3856b0-3eeb-11e8-a874-0050569b54a8   2G         RWX            Delete           Bound     default/my-scale-claim   spectrum-scale                  5s
```

Also you can see that related PV is created by scale-provisioner as a spectrum scale fileset and linked to the directory on the spectrum scale file system with specified quota size.

```console
$  /usr/lpp/mmfs/bin/mmlsfileset fs1
Filesets in file system 'fs1':
Name                     Status    Path
root                     Linked    /gpfs/fs1
user                     Linked    /gpfs/fs1/user
public                   Linked    /gpfs/fs1/public
admin                    Linked    /gpfs/fs1/admin
pvc-ff3856b0-3eeb-11e8-a874-0050569b54a8 Linked /gpfs/fs1/pvc-ff3856b0-3eeb-11e8-a874-0050569b54a8
```

```console
$ /usr/lpp/mmfs/bin/mmlsquota -j pvc-ff3856b0-3eeb-11e8-a874-0050569b54a8 fs1
                         Block Limits                                    |     File Limits
Filesystem type             KB      quota      limit   in_doubt    grace |    files   quota    limit in_doubt    grace  Remarks
fs1        FILESET           0    2097152    2097152          0     none |        1       0        0        0     none
```

Deploy an application that uses the new volume claim.

```console
$ cat > test-pod.yaml << EOF
kind: Pod
apiVersion: v1
metadata:
  name: test-pod
spec:
  containers:
  - name: test-pod
    image: "busybox"
    command:
      - sleep
      - "3600"
    volumeMounts:
      - name: spectrum-scale
        mountPath: "/mnt"
  volumes:
    - name: spectrum-scale
      persistentVolumeClaim:
        claimName: my-scale-claim
EOF
$ kubectl create -f test-pod.yaml
```

Now you can see PV on the spectrum scale file system is mounted to the container at `/mnt`. 

```console
$ kubectl exec -it test-pod sh
/ # df
Filesystem           1K-blocks      Used Available Use% Mounted on
overlay              1291175800  13748412 1211816432   1% /
tmpfs                    65536         0     65536   0% /dev
tmpfs                133844736         0 133844736   0% /sys/fs/cgroup
fs1                    2097152         0   2097152   0% /mnt
/dev/mapper/s824l--01--vg-root
                     1291175800  13748412 1211816432   1% /dev/termination-log
/dev/mapper/s824l--01--vg-root
                     1291175800  13748412 1211816432   1% /etc/resolv.conf
/dev/mapper/s824l--01--vg-root
                     1291175800  13748412 1211816432   1% /etc/hostname
/dev/mapper/s824l--01--vg-root
                     1291175800  13748412 1211816432   1% /etc/hosts
shm                      65536         0     65536   0% /dev/shm
tmpfs                133844736       192 133844544   0% /var/run/secrets/kubernetes.io/serviceaccount
tmpfs                    65536         0     65536   0% /proc/kcore
tmpfs                    65536         0     65536   0% /proc/timer_list
tmpfs                    65536         0     65536   0% /proc/timer_stats
tmpfs                    65536         0     65536   0% /proc/sched_debug
tmpfs                133844736         0 133844736   0% /sys/firmware
```

## Licensing

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.







