package informerfactory

import (
	"context"
	"errors"
	"fmt"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/haowenj/newcrd-api/api/v1beta1"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	virtv1 "kubevirt.io/api/core/v1"

	_ "kube-informer/informerfactory/scheme"
)

var (
	appLabel = "kubevirt.io=virt-launcher"
)

type newSharedInformer func() cache.SharedIndexInformer
type InformerFactory struct {
	log           logr.Logger
	informers     map[string]cache.SharedIndexInformer
	factory       informers.SharedInformerFactory
	lock          sync.Mutex
	k8sConfig     *rest.Config
	restClient    *rest.RESTClient
	clientSet     *kubernetes.Clientset
	defaultResync time.Duration
}

func NewInformerFactory(log logr.Logger, k8sConfig *rest.Config) *InformerFactory {
	informerFactory := &InformerFactory{
		log:           log,
		k8sConfig:     k8sConfig,
		defaultResync: time.Hour,
		informers:     make(map[string]cache.SharedIndexInformer),
	}
	//初始化restClient
	informerFactory.restClient, _ = rest.RESTClientFor(k8sConfig)
	//初始化ClientSet
	informerFactory.clientSet, _ = kubernetes.NewForConfig(k8sConfig)
	//初始化Informer工厂，运行所有的内置资源的Informer
	informerFactory.factory = informers.NewSharedInformerFactoryWithOptions(informerFactory.clientSet, 0)

	return informerFactory
}

// Start 运行所有的Informer
func (f *InformerFactory) Start(stopCh <-chan struct{}) {
	f.lock.Lock()
	defer f.lock.Unlock()

	//启动自定义资源的Informer
	for name, informer := range f.informers {
		f.log.Info("STARTING informer", "name", name)
		go informer.Run(stopCh)
	}
	//启动内置资源的Informer，factory启动便可以遍历启动所有的Informer
	f.factory.Start(stopCh)
}

// WaitForCacheSync 同步所有Informer的缓存数据
func (f *InformerFactory) WaitForCacheSync(stopCh <-chan struct{}) {
	var syncs []cache.InformerSynced

	f.lock.Lock()
	for name, informer := range f.informers {
		f.log.Info("Waiting for cache sync of informer", "name", name)
		syncs = append(syncs, informer.HasSynced)
	}
	f.lock.Unlock()

	//同步自定义资源的缓存数据
	cache.WaitForCacheSync(stopCh, syncs...)
	//同步内置资源的缓存数据
	f.factory.WaitForCacheSync(stopCh)
}

func (f *InformerFactory) ClientSet() *kubernetes.Clientset {
	return f.clientSet
}

func (f *InformerFactory) VirtualMachine() cache.SharedIndexInformer {
	groupVersion := &virtv1.StorageGroupVersion
	groupVersion.Version = virtv1.ApiLatestVersion
	restClient, _ := rest.RESTClientFor(f.resetK8sConf(groupVersion, scheme.Codecs))
	return f.getInformer("vmInformer", func() cache.SharedIndexInformer {
		lw := cache.NewListWatchFromClient(restClient, "virtualmachines", k8sv1.NamespaceAll, fields.Everything())
		//cache.Indexers参数的作用：informer.GetIndexer().ByIndex(cache.NamespaceIndex, "ucan-161")，可以根据命名空间的名字筛选数据
		return cache.NewSharedIndexInformer(lw, &virtv1.VirtualMachine{}, f.defaultResync, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	})
}

// AllPods 用于获取所有pod数据时使用的Informer
func (f *InformerFactory) AllPods() cache.SharedIndexInformer {
	return f.factory.Core().V1().Pods().Informer()
}

// AllPodsUseCustomIndexer 使用Indexer筛选缓存中的pod数据
func (f *InformerFactory) AllPodsUseCustomIndexer() cache.SharedIndexInformer {
	return f.getInformer("indexerPods", func() cache.SharedIndexInformer {
		lw := cache.NewListWatchFromClient(f.clientSet.CoreV1().RESTClient(), "pods", k8sv1.NamespaceAll, fields.Everything())
		//自定义Indexer的类型是一个字符串，加一个func(obj interface{}) ([]string, error)类型的函数，字符串就是Indexer的名称，根据这个名称就可以使用这个索引。
		//函数里的逻辑就是把缓存的数据按照一个维度进行分组，同时也可以筛选过滤掉一些数据，返回的字符串类型的切片，每个元素都是一个类似自定义的主键然后可以索引到背后的一个资源对象，这里是pod对象。
		return cache.NewSharedIndexInformer(lw, &k8sv1.Pod{}, f.defaultResync, cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
			"prometheus": func(obj interface{}) ([]string, error) {
				pod, ok := obj.(*k8sv1.Pod)
				if !ok {
					return []string{}, errors.New("invalid pod object")
				}
				var pods []string
				//这里以Prometheus的pod为例，先筛选所有存在app.kubernetes.io/part-of标签的pod，然后再根据app.kubernetes.io/component标签进行分组，用app.kubernetes.io/component=exporter的值
				//取不同的数据，在加上命名空间，用/连接。
				if _, ok := pod.GetLabels()["app.kubernetes.io/part-of"]; ok {
					pods = append(pods, fmt.Sprintf("%s/%s", pod.GetLabels()["app.kubernetes.io/component"], pod.Namespace))
				}
				return pods, nil
			},
		})
	})
}

// PortionPods 部分pod数据，根据标签筛选，跟AllPods选择使用，筛选数据的Informer需要自己实现listwatch接口才行
func (f *InformerFactory) PortionPods() cache.SharedIndexInformer {
	return f.getInformer("portionPods", func() cache.SharedIndexInformer {
		//这里的标签表达式，可以是一个key=value的形式，匹配标签和值，也可以只写一个标签，只写一个标签就是匹配所有带有这个标签的pod，不管值是啥。
		labelSelector, err := labels.Parse(appLabel)
		if err != nil {
			panic(err)
		}

		//k8sv1.NamespaceAll是一个空值，也就是说如果要拿所有命名空间下的数据或者说不进行命名空间的过滤，就可以在namespace里传入一个空值。
		lw := f.newListWatchFromClient(f.clientSet.CoreV1().RESTClient(), "pods", k8sv1.NamespaceAll, fields.Everything(), labelSelector)
		return cache.NewSharedIndexInformer(lw, &k8sv1.Pod{}, f.defaultResync, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	})
}

// NewCrd 获取自己开发的自定义资源newcrd的Informer
func (f *InformerFactory) NewCrd() cache.SharedIndexInformer {
	restClient, _ := rest.RESTClientFor(f.resetK8sConf(&v1beta1.GroupVersion, scheme.Codecs))
	return f.getInformer("newcrdInformer", func() cache.SharedIndexInformer {
		lw := cache.NewListWatchFromClient(restClient, "newdeps", k8sv1.NamespaceAll, fields.Everything())
		return cache.NewSharedIndexInformer(lw, &v1beta1.NewDep{}, f.defaultResync, cache.Indexers{})
	})
}

// 重置k8si信息，用于初始化每个自定义资源的Informer的restclinet
func (c *InformerFactory) resetK8sConf(gv *schema.GroupVersion, codecs serializer.CodecFactory) *rest.Config {
	shallowCopy := *c.k8sConfig
	shallowCopy.GroupVersion = gv
	shallowCopy.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: codecs}
	shallowCopy.APIPath = "/apis"
	shallowCopy.ContentType = runtime.ContentTypeJSON
	return &shallowCopy
}

func (f *InformerFactory) getInformer(key string, newFunc newSharedInformer) cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informer, exists := f.informers[key]
	if exists {
		return informer
	}
	informer = newFunc()
	f.informers[key] = informer

	return informer
}

// 创建自定义的listWatch实例
func (f *InformerFactory) newListWatchFromClient(c cache.Getter, resource string, namespace string, fieldSelector fields.Selector, labelSelector labels.Selector) *cache.ListWatch {
	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		options.FieldSelector = fieldSelector.String()
		options.LabelSelector = labelSelector.String()
		return c.Get().
			Namespace(namespace).
			Resource(resource).
			VersionedParams(&options, metav1.ParameterCodec).
			Do(context.Background()).
			Get()
	}
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		options.FieldSelector = fieldSelector.String()
		options.LabelSelector = labelSelector.String()
		options.Watch = true
		return c.Get().
			Namespace(namespace).
			Resource(resource).
			VersionedParams(&options, metav1.ParameterCodec).
			Watch(context.Background())
	}
	return &cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc}
}
