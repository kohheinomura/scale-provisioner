/*
Copyright 2018 KN.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
        "os"
        "os/exec"
	"path"
        "github.com/davecgh/go-spew/spew"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/external-storage/lib/controller"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"syscall"
)

const (
	provisionerName = "ccc.ibm.co.jp/scale"
)

type scaleProvisioner struct {
	pvDir string
        fsName string
//        identity string
}

func NewScaleProvisioner() controller.Provisioner {
        dir := os.Getenv("PV_DIR")
        name := os.Getenv("FS_NAME")
	return &scaleProvisioner{
		pvDir:    dir,
                fsName:   name,
//                identity: "worker1",
	}
}

var _ controller.Provisioner = &scaleProvisioner{}

// Provision creates a storage asset and returns a PV object representing it.
func (p *scaleProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {
        glog.Infof("scale provisioner: VolumeOptions: %s", spew.Sdump(options))
        path := path.Join(p.pvDir, options.PVName)

        // /usr/lpp/mmfs/bin/mmcrfileset <file system name> <fileset name>
        glog.Infof("scale provisioner: create fileset(cmd)   : " + "/usr/lpp/mmfs/bin/mmcrfileset " +  p.fsName + " " + options.PVName)
        out, err := exec.Command("/usr/lpp/mmfs/bin/mmcrfileset", p.fsName, options.PVName).Output()
        if err != nil {
                return nil, err
        }
        glog.Infof("scale provisioner: create fileset(result): %s", out)

        // /usr/lpp/mmfs/bin/mmlinkfileset <file system name> <fileset name> -J <target_dir>   
        glog.Infof("scale provisioner: link fileset(cmd)   : " + "/usr/lpp/mmfs/bin/mmlinkfileset " +  p.fsName + " " + options.PVName + " -J " + p.pvDir + "/" + options.PVName)
        out, err = exec.Command("/usr/lpp/mmfs/bin/mmlinkfileset", p.fsName, options.PVName, "-J", p.pvDir + "/" + options.PVName).Output()
        if err != nil {
                return nil, err
        }
        glog.Infof("scale provisioner: link fileset(result): %s", out)

        // /usr/lpp/mmfs/bin/mmsetquota <file system name>:<fileset name> --block <soft limit>:<hard limit>
        size := options.PVC.Spec.Resources.Requests["storage"]
        glog.Infof("scale provisioner: set quota(cmd)   : " + "/usr/lpp/mmfs/bin/mmsetquota " +  p.fsName + ":" + options.PVName + " --block " +  size.String() + ":" + size.String())
        err = exec.Command("/usr/lpp/mmfs/bin/mmsetquota", p.fsName + ":" + options.PVName, "--block",  size.String() + ":" + size.String()).Run()
        if err != nil {
                return nil, err
        }

	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
//			Annotations: map[string]string{
//				"hostPathProvisionerIdentity": p.identity,
//			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: path,
				},
			},
		},
	}

	return pv, nil
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *scaleProvisioner) Delete(volume *v1.PersistentVolume) error {

        // /usr/lpp/mmfs/bin/mmunlinkfileset <file system name> <fileset name>
        glog.Infof("scale provisioner: unlink fileset(cmd)   : " + "/usr/lpp/mmfs/bin/mmunlinkfileset " +  p.fsName + " " + volume.Name)
        out, err := exec.Command("/usr/lpp/mmfs/bin/mmunlinkfileset", p.fsName, volume.Name).Output()
        if err != nil {
                return err
        }
        glog.Infof("scale provisioner: unlink fileset(result): %s", out)

        // /usr/lpp/mmfs/bin/mmdelfileset <file system name> <fileset name>
        glog.Infof("scale provisioner: del fileset(cmd)   : " + "/usr/lpp/mmfs/bin/mmdelfileset " +  p.fsName + " " + volume.Name)
        out, err = exec.Command("/usr/lpp/mmfs/bin/mmdelfileset", p.fsName, volume.Name).Output()
        if err != nil {
                return err
        }
        glog.Infof("scale provisioner: del fileset(result): %s", out)

	return nil
}

func main() {
	syscall.Umask(0)

	flag.Parse()
	flag.Set("logtostderr", "true")

	// Create an InClusterConfig and use it to create a client for the controller
	// to use to communicate with Kubernetes
	config, err := rest.InClusterConfig()
	if err != nil {
		glog.Fatalf("Failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create client: %v", err)
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		glog.Fatalf("Error getting server version: %v", err)
	}

	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	scaleProvisioner := NewScaleProvisioner()

	// Start the provision controller which will dynamically provision hostPath
	// PVs
	pc := controller.NewProvisionController(clientset, provisionerName, scaleProvisioner, serverVersion.GitVersion)
	pc.Run(wait.NeverStop)
}
