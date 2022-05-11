package main

import (
	"flag"
	"time"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	clientset "github.com/Khighness/highness-network-controller/pkg/client/clientset/versioned"
	informers "github.com/Khighness/highness-network-controller/pkg/client/informers/externalversions"
	"github.com/Khighness/highness-network-controller/pkg/signals"
)

// @Author Chen Zikang
// @Email  zikang.chen@shopee.com
// @Since  2022-05-10

var (
	masterURL  string
	kubeConfig string
)

func init() {
	flag.StringVar(&masterURL, "master", "", "The address of the kubernetes API server. Overrides any value in kubeConfig. Only required if out-of-cluster.")
	flag.StringVar(&kubeConfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
}

func main() {
	flag.Parse()

	// handler the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	// 1.1 build config via the url of APIServer and the path of kubeConfig
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeConfig)
	if err != nil {
		glog.Fatalf("Error building kubeConfig: %s", err.Error())
	}
	// 1.2 create the client of kubernetes
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientSet: %s", err.Error())
	}
	// 1.3 create the client of network
	networkClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building example clientSet: %S", err.Error())
	}

	// 2. create a factory for network, and transfer if to controller
	networkInformerFactory := informers.NewSharedInformerFactory(networkClient, 30*time.Second)
	controller := NewController(kubeClient, networkClient, networkInformerFactory.Samplecrd().V1().Networks())

	// 3. start the factory and the controller
	go networkInformerFactory.Start(stopCh)
	if err = controller.Run(2, stopCh); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}
}
