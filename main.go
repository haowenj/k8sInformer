package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	k8sv1 "k8s.io/api/core/v1"
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
	//获取所有的pod数据
	allPodInformer := factory.PortionPods()

	//运行所有的Informer
	factory.Start(ctx.Done())
	//缓存数据
	factory.WaitForCacheSync(ctx.Done())

	//使用Informer处理业务逻辑，logger.WithName方法会返回一个全新的logger实例，并且打印的日志内容会加上Name前缀
	go runVmInformer(vmInformer, logger.WithName("vm informer"))
	go runAllPodInformer(allPodInformer, logger.WithName("pod informer"))

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

func runAllPodInformer(informer cache.SharedIndexInformer, logs logr.Logger) {
	//list
	fun := func() {
		objs := informer.GetStore().List()
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
