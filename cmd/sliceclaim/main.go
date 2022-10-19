package main

import (
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/EdgeNet-project/edgenet/pkg/bootstrap"
	"github.com/EdgeNet-project/edgenet/pkg/controller/core/v1alpha1/sliceclaim"
	informers "github.com/EdgeNet-project/edgenet/pkg/generated/informers/externalversions"
	"github.com/EdgeNet-project/edgenet/pkg/signals"

	"k8s.io/klog"
)

func main() {
	klog.InitFlags(nil)
	flag.String("kubeconfig-path", bootstrap.GetDefaultKubeconfigPath(), "Path to the kubeconfig file's directory")
	provisioning := flag.String("provisioning", "Dynamic", "Working mode to automate slice creation")
	flag.Parse()

	stopCh := signals.SetupSignalHandler()
	var authentication string
	if authentication := strings.TrimSpace(os.Getenv("AUTHENTICATION_STRATEGY")); authentication == "" {
		authentication = "serviceaccount"
	}
	kubeclientset, err := bootstrap.CreateClientset(authentication)
	if err != nil {
		log.Println(err.Error())
		panic(err.Error())
	}
	edgenetclientset, err := bootstrap.CreateEdgeNetClientset(authentication)
	if err != nil {
		log.Println(err.Error())
		panic(err.Error())
	}

	// Start the controller to provide the functionalities of sliceclaim resource
	edgenetInformerFactory := informers.NewSharedInformerFactory(edgenetclientset, time.Second*30)

	controller := sliceclaim.NewController(kubeclientset,
		edgenetclientset,
		edgenetInformerFactory.Core().V1alpha1().SubNamespaces(),
		edgenetInformerFactory.Core().V1alpha1().SliceClaims(),
		*provisioning)

	edgenetInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}
