package pod

import (
	"context"
	"embed"
	"fmt"
	"os"
	"strings"
	"text/template"

	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	"github.com/kubernetes-sigs/kernel-module-management/internal/api"
	"github.com/kubernetes-sigs/kernel-module-management/internal/constants"
	"github.com/mitchellh/hashstructure/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TemplateData struct {
	FilesToSign   []string
	SignImage     string
	UnsignedImage string
}

//go:embed templates
var templateFS embed.FS

var tmpl = template.Must(
	template.ParseFS(templateFS, "templates/Dockerfile.gotmpl"),
)

func (bsrm *buildSignResourceManager) buildSpec(mld *api.ModuleLoaderData, destinationImg string, pushImage bool) v1.PodSpec {

	buildConfig := mld.Build

	args := containerArgs(mld, destinationImg, mld.Build.BaseImageRegistryTLS, pushImage)
	overrides := []kmmv1beta1.BuildArg{
		{Name: "KERNEL_VERSION", Value: mld.KernelVersion},
		{Name: "KERNEL_FULL_VERSION", Value: mld.KernelVersion},
		{Name: "MOD_NAME", Value: mld.Name},
		{Name: "MOD_NAMESPACE", Value: mld.Namespace},
	}
	buildArgs := bsrm.combiner.ApplyBuildArgOverrides(
		buildConfig.BuildArgs,
		overrides...,
	)
	for _, ba := range buildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", ba.Name, ba.Value))
	}

	kanikoImage := os.Getenv("RELATED_IMAGE_BUILD")

	if buildConfig.KanikoParams != nil && buildConfig.KanikoParams.Tag != "" {
		if idx := strings.IndexAny(kanikoImage, "@:"); idx != -1 {
			kanikoImage = kanikoImage[0:idx]
		}

		kanikoImage += ":" + buildConfig.KanikoParams.Tag
	}

	selector := mld.Selector
	if len(mld.Build.Selector) != 0 {
		selector = mld.Build.Selector
	}

	volumes, volumeMounts := makeBuildResourceVolumesAndVolumeMounts(*buildConfig, mld.ImageRepoSecret)

	return v1.PodSpec{
		Containers: []v1.Container{
			{
				Args:         args,
				Name:         "kaniko",
				Image:        kanikoImage,
				VolumeMounts: volumeMounts,
			},
		},
		RestartPolicy: v1.RestartPolicyNever,
		Volumes:       volumes,
		NodeSelector:  selector,
		Tolerations:   mld.Tolerations,
	}
}

func signSpec(mld *api.ModuleLoaderData, destinationImg string, pushImage bool) v1.PodSpec {

	signConfig := mld.Sign
	args := containerArgs(mld, destinationImg, signConfig.UnsignedImageRegistryTLS, pushImage)
	volumes, volumeMounts := makeSignResourceVolumesAndVolumeMounts(signConfig, mld.ImageRepoSecret)

	return v1.PodSpec{
		Containers: []v1.Container{
			{
				Args:         args,
				Name:         "kaniko",
				Image:        os.Getenv("RELATED_IMAGE_BUILD"),
				VolumeMounts: volumeMounts,
			},
		},
		RestartPolicy: v1.RestartPolicyNever,
		Volumes:       volumes,
		NodeSelector:  mld.Selector,
		Tolerations:   mld.Tolerations,
	}
}

func containerArgs(mld *api.ModuleLoaderData, destinationImg string,
	tlsOptions kmmv1beta1.TLSOptions, pushImage bool) []string {

	args := []string{}

	if pushImage {
		args = append(args, "--destination", destinationImg)
		if mld.RegistryTLS.Insecure {
			args = append(args, "--insecure")
		}
		if mld.RegistryTLS.InsecureSkipTLSVerify {
			args = append(args, "--skip-tls-verify")
		}
	} else {
		args = append(args, "--no-push")
	}

	if tlsOptions.Insecure {
		args = append(args, "--insecure-pull")
	}

	if tlsOptions.InsecureSkipTLSVerify {
		args = append(args, "--skip-tls-verify-pull")
	}

	return args

}

func (bsrm *buildSignResourceManager) getBuildHashAnnotationValue(ctx context.Context, configMapName, namespace string,
	buildSpec *v1.PodSpec) (uint64, error) {

	dockerfileCM := &v1.ConfigMap{}
	namespacedName := types.NamespacedName{Name: configMapName, Namespace: namespace}
	if err := bsrm.client.Get(ctx, namespacedName, dockerfileCM); err != nil {
		return 0, fmt.Errorf("failed to get dockerfile ConfigMap %s: %v", namespacedName, err)
	}
	dockerfile, ok := dockerfileCM.Data[constants.DockerfileCMKey]
	if !ok {
		return 0, fmt.Errorf("invalid Dockerfile ConfigMap %s format, %s key is missing", namespacedName, constants.DockerfileCMKey)
	}

	dataToHash := struct {
		buildSpec  *v1.PodSpec
		Dockerfile string
	}{
		buildSpec:  buildSpec,
		Dockerfile: dockerfile,
	}
	hashValue, err := hashstructure.Hash(dataToHash, hashstructure.FormatV2, nil)
	if err != nil {
		return 0, fmt.Errorf("could not hash build's spec template and dockefile: %v", err)
	}

	return hashValue, nil
}

func (bsrm *buildSignResourceManager) getSignHashAnnotationValue(ctx context.Context, privateSecret, publicSecret, namespace string,
	signConfig []byte, signSpec *v1.PodSpec) (uint64, error) {

	privateKeyData, err := bsrm.getSecretData(ctx, privateSecret, constants.PrivateSignDataKey, namespace)
	if err != nil {
		return 0, fmt.Errorf("failed to get private secret %s for signing: %v", privateSecret, err)
	}
	publicKeyData, err := bsrm.getSecretData(ctx, publicSecret, constants.PublicSignDataKey, namespace)
	if err != nil {
		return 0, fmt.Errorf("failed to get public secret %s for signing: %v", publicSecret, err)
	}

	dataToHash := struct {
		SignSpec       *v1.PodSpec
		PrivateKeyData []byte
		PublicKeyData  []byte
		SignConfig     []byte
	}{
		SignSpec:       signSpec,
		PrivateKeyData: privateKeyData,
		PublicKeyData:  publicKeyData,
		SignConfig:     signConfig,
	}
	hashValue, err := hashstructure.Hash(dataToHash, hashstructure.FormatV2, nil)
	if err != nil {
		return 0, fmt.Errorf("could not hash sign's spec template and signing keys: %v", err)
	}

	return hashValue, nil
}

func (bsrm *buildSignResourceManager) getSecretData(ctx context.Context, secretName, secretDataKey, namespace string) ([]byte, error) {
	secret := v1.Secret{}
	namespacedName := types.NamespacedName{Name: secretName, Namespace: namespace}
	err := bsrm.client.Get(ctx, namespacedName, &secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get Secret %s: %v", namespacedName, err)
	}
	data, ok := secret.Data[secretDataKey]
	if !ok {
		return nil, fmt.Errorf("invalid Secret %s format, %s key is missing", namespacedName, secretDataKey)
	}
	return data, nil
}

func resourceLabels(modName, targetKernel string, resourceType kmmv1beta1.BuildOrSignAction) map[string]string {

	labels := moduleKernelLabels(modName, targetKernel, resourceType)

	labels["app.kubernetes.io/name"] = "kmm"
	labels["app.kubernetes.io/component"] = string(resourceType)
	labels["app.kubernetes.io/part-of"] = "kmm"

	return labels
}

func filterResourcesByOwner(resources []v1.Pod, owner metav1.Object) []v1.Pod {
	ownedResources := []v1.Pod{}
	for _, obj := range resources {
		if metav1.IsControlledBy(&obj, owner) {
			ownedResources = append(ownedResources, obj)
		}
	}
	return ownedResources
}

func moduleKernelLabels(moduleName, targetKernel string, resourceType kmmv1beta1.BuildOrSignAction) map[string]string {
	labels := moduleLabels(moduleName, resourceType)
	labels[constants.TargetKernelTarget] = targetKernel
	return labels
}

func moduleLabels(moduleName string, resourceType kmmv1beta1.BuildOrSignAction) map[string]string {
	return map[string]string{
		constants.ModuleNameLabel: moduleName,
		constants.ResourceType:    string(resourceType),
	}
}

func (bsrm *buildSignResourceManager) getResources(ctx context.Context, namespace string, labels map[string]string) ([]v1.Pod, error) {
	resourceList := v1.PodList{}
	opts := []client.ListOption{
		client.MatchingLabels(labels),
		client.InNamespace(namespace),
	}
	if err := bsrm.client.List(ctx, &resourceList, opts...); err != nil {
		return nil, fmt.Errorf("could not list resources: %v", err)
	}

	return resourceList.Items, nil
}
