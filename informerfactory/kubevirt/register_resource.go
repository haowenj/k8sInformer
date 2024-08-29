package kubevirt

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"kubevirt.io/api/core"
	virtv1 "kubevirt.io/api/core/v1"
)

var (
	SchemeBuilder  runtime.SchemeBuilder
	Scheme         *runtime.Scheme
	Codecs         serializer.CodecFactory
	ParameterCodec runtime.ParameterCodec
)

func init() {
	registerVersion := os.Getenv(virtv1.KubeVirtClientGoSchemeRegistrationVersionEnvVar)
	if registerVersion != "" {
		SchemeBuilder = runtime.NewSchemeBuilder(virtv1.AddKnownTypesGenerator([]schema.GroupVersion{{Group: core.GroupName, Version: registerVersion}}))
	} else {
		SchemeBuilder = runtime.NewSchemeBuilder(virtv1.AddKnownTypesGenerator(virtv1.GroupVersions))
	}
	Scheme = runtime.NewScheme()
	AddToScheme := SchemeBuilder.AddToScheme
	Codecs = serializer.NewCodecFactory(Scheme)
	ParameterCodec = runtime.NewParameterCodec(Scheme)
	utilruntime.Must(AddToScheme(Scheme))
	utilruntime.Must(AddToScheme(scheme.Scheme))
	fmt.Println("*********** kubevirt client go scheme ***********")
}
