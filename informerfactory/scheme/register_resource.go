package newScheme

import (
	"fmt"

	"github.com/haowenj/newcrd-api/api/v1beta1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
)

/*
*
一般情况下引入自定义资源的api结构体定义时，在他们的源码包里的groupversion_info.go文件里都已经定义好了AddToScheme变量，可以直接使用utilrunt ime.Must方法注入到当前的Client-gos实例中。
这样当前的client-gos实例中的客户端就能知道有这么个新的gv了。
*/
func init() {
	utilruntime.Must(v1beta1.AddToScheme(scheme.Scheme))
	utilruntime.Must(virtv1.AddToScheme(scheme.Scheme))
	fmt.Println("*********** register client go scheme ***********")
}
