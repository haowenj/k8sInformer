package main

import (
	"context"
	"flag"
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
	//启动newCrd Informer
	newcrdInformer := factory.NewCrd()

	//运行所有的Informer
	factory.Start(ctx.Done())
	//缓存数据
	factory.WaitForCacheSync(ctx.Done())

	//使用Informer处理业务逻辑，logger.WithName方法会返回一个全新的logger实例，并且打印的日志内容会加上Name前缀
	go runVmInformer(vmInformer, logger.WithName("vm informer"))
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
