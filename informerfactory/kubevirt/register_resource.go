package kubevirt

import (
	"fmt"
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

func init() {
	Scheme = runtime.NewScheme()
	Codecs = serializer.NewCodecFactory(Scheme)
	utilruntime.Must(virtv1.AddToScheme(Scheme))
	utilruntime.Must(virtv1.AddToScheme(scheme.Scheme))
	fmt.Println("*********** kubevirt client go scheme ***********")
}
