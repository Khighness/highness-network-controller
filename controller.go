package main

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	samplecrdv1 "github.com/Khighness/highness-network-controller/pkg/apis/samplecrd/v1"
	clientset "github.com/Khighness/highness-network-controller/pkg/client/clientset/versioned"
	networkschema "github.com/Khighness/highness-network-controller/pkg/client/clientset/versioned/scheme"
	informers "github.com/Khighness/highness-network-controller/pkg/client/informers/externalversions/samplecrd/v1"
	listers "github.com/Khighness/highness-network-controller/pkg/client/listers/samplecrd/v1"
)

// @Author Chen Zikang
// @Email  zikang.chen@shopee.com
// @Since  2022-05-10

const controllerAgentName = "network-controller"

const (
	SuccessSynced         = "Synced"
	MessageResourceSynced = "Network synced successfully"
)

type Controller struct {
	// kubeClientSet is a standard kubernetes clientSet
	kubeClientSet kubernetes.Interface
	// networkClientSet is a clientSet for our own API group
	networkClientSet clientset.Interface

	networkListener listers.NetworkLister
	networkSynced   cache.InformerSynced

	// workQueue is a rate limited work queue used to queue work to
	// be processed instead of performing it as soon as a change happens
	workQueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to
	// the Kubernetes API
	recorder record.EventRecorder
}

func NewController(
	kubeClientSet kubernetes.Interface,
	networkClientSet clientset.Interface,
	networkInformer informers.NetworkInformer) *Controller {

	runtime.Must(networkschema.AddToScheme(scheme.Scheme))
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClientSet.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeClientSet:    kubeClientSet,
		networkClientSet: networkClientSet,
		networkListener:  networkInformer.Lister(),
		networkSynced:    networkInformer.Informer().HasSynced,
		workQueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Networks"),
		recorder:         recorder,
	}

	glog.Info("Setting up event handlers")
	networkInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueNetwork,
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNetwork := oldObj.(*samplecrdv1.Network)
			newNetwork := newObj.(*samplecrdv1.Network)
			if oldNetwork.ResourceVersion == newNetwork.ResourceVersion {
				return
			}
			controller.enqueueNetwork(newObj)
		},
		DeleteFunc: controller.enqueueNetworkForDelete,
	})

	return controller
}

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workQueue.ShutDown()

	// start the informer factories to begin populating the informer caches
	glog.Info("Starting network control loop")

	// wait for the caches to be synced before starting workers
	glog.Info("Waiting for informers caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.networkSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	// launch two workers to process network resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process e message
// on the workQueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workQueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workQueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workQueue.Done(obj)
		var (
			key string
			ok  bool
		)
		if key, ok = obj.(string); !ok {
			c.workQueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workQueue but get %#v", obj))
			return nil
		}
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		c.workQueue.Forget(obj)
		glog.Infof("Successfully sync '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the status block of the network resource
// with the current status of the resource
func (c *Controller) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(err)
		return nil
	}

	network, err := c.networkListener.Networks(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			glog.Warningf("Network: %s/%s does not exist in local cache, will delete it from Neutron ...",
				namespace, name)
			glog.Infof("[Neutron] Deleting network: %s/%s ...", namespace, name)
			return nil
		}
		runtime.HandleError(fmt.Errorf("failed to list nerwork by: %s/%s", namespace, name))
		return err
	}

	glog.Infof("[Neutron] Try to process network: %#v ...", network)
	c.recorder.Event(network, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

// enqueueNetwork takes a network resource and converts it into a namespace/name
// string which is then put onto the workQueue. This method should not be passed
// resources of any type other than network.
func (c *Controller) enqueueNetwork(obj interface{}) {
	var (
		key string
		err error
	)
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.workQueue.AddRateLimited(key)
}

// enqueueNetworkForDelete takes a deleted Network resource and converts it
// into a namespace/name  string which is then put onto the workQueue. This
// method should *not be passed resources of any type other than Network.
func (c *Controller) enqueueNetworkForDelete(obj interface{}) {
	var (
		key string
		err error
	)
	if key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.workQueue.AddRateLimited(key)
}
