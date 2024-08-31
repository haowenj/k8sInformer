package newScheme

import (
	"fmt"

	"github.com/haowenj/newcrd-api/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
)

var (
	Scheme *runtime.Scheme
	Codecs serializer.CodecFactory
)

/*
*
一般情况下引入自定义资源的api结构体定义时，在他们的源码包里的groupversion_info.go文件里都已经定义好了AddToScheme变量，可以直接使用utilrunt ime.Must方法注入到当前的Client-gos实例中。
这样当前的client-gos实例中的客户端就能知道有这么个新的gv了。
因为Informer需要一个restClient的客户端，这个客户端还需要一个codes来序列化反序列化结构，这个一般在源码包里是没有的，需要自己添加。也就是下边代码里的Codecs = serializer.NewCodecFactory(Scheme)部分。
当然注入一个新的gv时需要初始化一个新的Scheme，就是下边第一行代码，Scheme = runtime.NewScheme()。
*/
func init() {
	Scheme = runtime.NewScheme()
	Codecs = serializer.NewCodecFactory(Scheme)
	utilruntime.Must(v1beta1.AddToScheme(Scheme))
	utilruntime.Must(v1beta1.AddToScheme(scheme.Scheme))
	utilruntime.Must(virtv1.AddToScheme(Scheme))
	utilruntime.Must(virtv1.AddToScheme(scheme.Scheme))
	fmt.Println("*********** register client go scheme ***********")
}
