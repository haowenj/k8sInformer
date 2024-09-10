package main

import (
	"context"
	"flag"
	"fmt"
	"k8s.io/client-go/kubernetes"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/haowenj/newcrd-api/api/v1beta1"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"kubevirt.io/api/core/v1"

	"kube-informer/informerfactory"
	"kube-informer/pkg/log"
)

var (
	logger     = log.NewStdoutLogger()
	ctx        = context.Background()
	kubeconfig string
)

func init() {
	var defaultKubeConfigPath string
	if home := homedir.HomeDir(); home != "" {
		defaultKubeConfigPath = filepath.Join(home, ".kube", "config")
	}
	flag.StringVar(&kubeconfig, "kubeconfig", defaultKubeConfigPath, "absolute path to the kubeconfig file")
	flag.Parse()
}

func main() {
	k8sConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		logger.Error(err, "Read kubeconfig file", "path", kubeconfig)
		return
	}
	//初始化Informer工厂
	factory := informerfactory.NewInformerFactory(logger, k8sConfig)

	//启动虚拟机Informer
	vmInformer := factory.VirtualMachine()
	//启动pod Informer
	podInformer := factory.PortionPods()
	//启动带有自定义Indexer的pod Informer
	indexerPodInformer := factory.AllPodsUseCustomIndexer()
	//启动newCrd Informer
	newcrdInformer := factory.NewCrd()

	//运行所有的Informer
	factory.Start(ctx.Done())
	//缓存数据
	factory.WaitForCacheSync(ctx.Done())

	//使用Informer处理业务逻辑，logger.WithName方法会返回一个全新的logger实例，并且打印的日志内容会加上Name前缀
	go runVmInformer(vmInformer, logger.WithName("vm informer"))
	go runVmInformerUseIndexer(vmInformer, logger.WithName("vm informer use-indexer"))
	go runPodInformerUseIndexer(indexerPodInformer, logger.WithName("pod informer use-indexer"))
	go runAPodInformer(podInformer, logger.WithName("pod informer"))
	go runNewcrdInformer(newcrdInformer, factory.ClientSet(), logger.WithName("newdep informer"))

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	<-quit
	logger.Info("Shutdown Server ...")
}

func runVmInformer(informer cache.SharedIndexInformer, logs logr.Logger) {
	//watch
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			vm, ok := obj.(*v1.VirtualMachine)
			if !ok {
				logs.Error(nil, "unexpected type: %T", obj)
				return
			}
			logs.Info("add vm", "name", vm.Name, "namespace", vm.Namespace)
		},
		UpdateFunc: func(oldObj, newObj any) {
			vm, ok := newObj.(*v1.VirtualMachine)
			if !ok {
				logs.Error(nil, "unexpected type: %T", newObj)
				return
			}
			logs.Info("update vm", "name", vm.Name, "namespace", vm.Namespace)
		},
		DeleteFunc: func(obj any) {
			vm, ok := obj.(*v1.VirtualMachine)
			if !ok {
				logs.Error(nil, "unexpected type: %T", obj)
				return
			}
			logs.Info("delete vm", "name", vm.Name, "namespace", vm.Namespace)
		},
	})
	if err != nil {
		logs.Error(err, "Add EventHandler error")
		return
	}

	//list
	fun := func() {
		for _, obj := range informer.GetStore().List() {
			vm, ok := obj.(*v1.VirtualMachine)
			if !ok {
				logs.Error(nil, "unexpected type: %T", obj)
				continue
			}
			logs.Info("vm info:", vm.Namespace, vm.Name)
		}
	}
	wait.Until(fun, time.Hour, ctx.Done())
}

func runVmInformerUseIndexer(informer cache.SharedIndexInformer, logs logr.Logger) {
	//创建Informer的时候，传入的indexer参数的作用是把缓存的数据组建一个索引，根据索引可以从缓存数据里检索自己想要的数据，Informer默认提供了一个namespace的indexer，他的作用是按照命名空间建立索引，然后传入命名空间
	//就可以筛选指定命名空间下的数据，他跟自定义listwatch方法，自定标签筛选监听和缓存数据的区别是，前者数据仍然是全量的，只是缓存后再进行数据筛选，后者是直接在缓存和监听数据时就已经做了数据的筛选。
	objs, err := informer.GetIndexer().ByIndex(cache.NamespaceIndex, "ucan-161")
	if err != nil {
		logs.Error(err, "get indexer error")
		return
	}
	for _, obj := range objs {
		vm, ok := obj.(*v1.VirtualMachine)
		if !ok {
			logs.Error(nil, "unexpected type: %T", obj)
			continue
		}
		logs.Info("vm info:", vm.Namespace, vm.Name)
	}
}

func runPodInformerUseIndexer(informer cache.SharedIndexInformer, logs logr.Logger) {
	//使用自定义Indexer,prometheus是创建Informer时是指定的Indexer的名字，值是kube-Prometheus项目部署时，依据组建名称打的不同的标签值然后加上命名空间
	objs, err := informer.GetIndexer().ByIndex("prometheus", "prometheus/monitoring")
	if err != nil {
		logs.Error(err, "get indexer error")
		return
	}
	//ListIndexFuncValues方法传入一个索引器的名字，返回的是这个索引器下的所有的索引值
	for key, val := range informer.GetIndexer().ListIndexFuncValues("prometheus") {
		fmt.Printf("ListIndexFuncValues: %v: %v\n", key, val)
	}
	//GetIndexers方法获取Informer里所有的索引器以及对应的函数
	for key, val := range informer.GetIndexer().GetIndexers() {
		fmt.Printf("GetIndexers: %v: %v\n", key, val)
	}
	//IndexKeys方法获取根据索引器以及索引值获得的资源的key，而不是具体的资源对象
	keys, _ := informer.GetIndexer().IndexKeys("prometheus", "prometheus/monitoring")
	for key, val := range keys {
		fmt.Printf("IndexKeys: %v: %v\n", key, val)
	}
	for key, obj := range objs {
		pod, ok := obj.(*k8sv1.Pod)
		if !ok {
			logs.Error(nil, "unexpected type: %T", obj)
			continue
		}
		logs.Info("pod info:", "key", key, pod.Namespace, pod.Name)
	}
}

func runAPodInformer(informer cache.SharedIndexInformer, logs logr.Logger) {
	//list
	fun := func() {
		objs := informer.GetStore().List()
		//pod在本项目中有两个Informer，一个是获取全量Informer的，为了防止启动时输出大量pod数据，限制只输出5个，另一个是根据标签筛选后的Informer，可以忽略下方这个逻辑，当然标签筛选后也可能
		//会有很多数据，防止启动时打印太多数据，应用于实际开发中时要删除这个逻辑。
		if len(objs) > 5 {
			objs = objs[:5]
		}
		for _, obj := range objs {
			pod, ok := obj.(*k8sv1.Pod)
			if !ok {
				logs.Error(nil, "unexpected type: %T", obj)
				return
			}
			logs.Info("pod info", pod.Namespace, pod.Name)
		}
	}
	wait.Until(fun, time.Hour, ctx.Done())
}

func runNewcrdInformer(informer cache.SharedIndexInformer, clientSet *kubernetes.Clientset, logs logr.Logger) {
	//list
	fun := func() {
		objs := informer.GetStore().List()
		for _, obj := range objs {
			newdep, ok := obj.(*v1beta1.NewDep)
			if !ok {
				logs.Error(nil, "unexpected type: %T", obj)
				return
			}
			logs.Info("newdep info", newdep.Namespace, newdep.Name)
			//根据父级cr查询所管理的pod，父级查pod只能通过标签的方式，但是pod对象里有个pod.GetOwnerReferences()方法，可以直接查pod属于谁管理。
			selector := metav1.LabelSelector{MatchLabels: map[string]string{"app": newdep.Name}}
			pods, err := clientSet.CoreV1().Pods(newdep.Namespace).List(ctx, metav1.ListOptions{LabelSelector: metav1.FormatLabelSelector(&selector)})
			if err != nil {
				logs.Error(err, "get pod list")
				return
			}
			for _, pod := range pods.Items {
				for _, owner := range pod.GetOwnerReferences() {
					logs.Info("owner", "name", owner.Name)
				}
				logs.Info("pod info", pod.Namespace, pod.Name)
			}
		}
	}
	wait.Until(fun, time.Hour, ctx.Done())
}
