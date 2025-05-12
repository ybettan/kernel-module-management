package pod

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	"github.com/kubernetes-sigs/kernel-module-management/internal/api"
	"github.com/kubernetes-sigs/kernel-module-management/internal/buildsign"
	"github.com/kubernetes-sigs/kernel-module-management/internal/constants"
	"github.com/kubernetes-sigs/kernel-module-management/internal/module"
)

const (
	dockerfileAnnotationKey = "dockerfile"
	dockerfileVolumeName    = "dockerfile"
)

type buildSignResourceManager struct {
	client   client.Client
	combiner module.Combiner
	scheme   *runtime.Scheme
}

func NewBuildSignResourceManager(client client.Client, combiner module.Combiner,
	scheme *runtime.Scheme) buildsign.BuildSignResourceManager {

	return &buildSignResourceManager{
		client:   client,
		combiner: combiner,
		scheme:   scheme,
	}
}

func (bsrm *buildSignResourceManager) MakeBuildResourceTemplate(ctx context.Context, mld *api.ModuleLoaderData, owner metav1.Object,
	pushImage bool) (runtime.Object, error) {

	// if build AND sign are specified, then we will build an intermediate image
	// and let sign produce the one specified in its targetImage
	containerImage := mld.ContainerImage
	if module.ShouldBeSigned(mld) {
		containerImage = module.IntermediateImageName(mld.Name, mld.Namespace, containerImage)
	}

	buildSpec := bsrm.buildSpec(mld, containerImage, pushImage)
	buildSpecHash, err := bsrm.getBuildHashAnnotationValue(
		ctx,
		mld.Build.DockerfileConfigMap.Name,
		mld.Namespace,
		&buildSpec,
	)
	if err != nil {
		return nil, fmt.Errorf("could not hash pod's definitions: %v", err)
	}

	build := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mld.Name + "-build-",
			Namespace:    mld.Namespace,
			Labels:       resourceLabels(mld.Name, mld.KernelNormalizedVersion, kmmv1beta1.BuildImage),
			Annotations:  map[string]string{constants.PodHashAnnotation: fmt.Sprintf("%d", buildSpecHash)},
			Finalizers:   []string{constants.GCDelayFinalizer, constants.JobEventFinalizer},
		},
		Spec: buildSpec,
	}

	if err := controllerutil.SetControllerReference(owner, build, bsrm.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return build, nil
}

func (bsrm *buildSignResourceManager) MakeSignResourceTemplate(ctx context.Context, mld *api.ModuleLoaderData, owner metav1.Object,
	pushImage bool) (runtime.Object, error) {

	signConfig := mld.Sign

	var buf bytes.Buffer

	td := TemplateData{
		FilesToSign: mld.Sign.FilesToSign,
		SignImage:   os.Getenv("RELATED_IMAGE_SIGN"),
	}

	imageToSign := ""
	if module.ShouldBeBuilt(mld) {
		imageToSign = module.IntermediateImageName(mld.Name, mld.Namespace, mld.ContainerImage)
	}

	if imageToSign != "" {
		td.UnsignedImage = imageToSign
	} else if signConfig.UnsignedImage != "" {
		td.UnsignedImage = signConfig.UnsignedImage
	} else {
		return nil, fmt.Errorf("no image to sign given")
	}

	if err := tmpl.Execute(&buf, td); err != nil {
		return nil, fmt.Errorf("could not execute template: %v", err)
	}

	signSpec := signSpec(mld, mld.ContainerImage, pushImage)
	signSpecHash, err := bsrm.getSignHashAnnotationValue(ctx, signConfig.KeySecret.Name,
		signConfig.CertSecret.Name, mld.Namespace, buf.Bytes(), &signSpec)
	if err != nil {
		return nil, fmt.Errorf("could not hash pod's definitions: %v", err)
	}

	sign := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mld.Name + "-sign-",
			Namespace:    mld.Namespace,
			Labels:       resourceLabels(mld.Name, mld.KernelNormalizedVersion, kmmv1beta1.SignImage),
			Annotations: map[string]string{
				constants.PodHashAnnotation: fmt.Sprintf("%d", signSpecHash),
				dockerfileAnnotationKey:     buf.String(),
			},
			Finalizers: []string{constants.GCDelayFinalizer, constants.JobEventFinalizer},
		},
		Spec: signSpec,
	}

	if err = controllerutil.SetControllerReference(owner, sign, bsrm.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return sign, nil
}

func (bsrm *buildSignResourceManager) CreateBuildSignResource(ctx context.Context, obj runtime.Object) error {

	resource, ok := obj.(*v1.Pod)
	if !ok {
		return errors.New("the resource cannot be converted to the corect resource")
	}
	err := bsrm.client.Create(ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

func (bsrm *buildSignResourceManager) DeleteBuildSignResource(ctx context.Context, obj runtime.Object) error {

	resource, ok := obj.(*v1.Pod)
	if !ok {
		return errors.New("the resource cannot be converted to the correct resource")
	}
	opts := []client.DeleteOption{
		client.PropagationPolicy(metav1.DeletePropagationBackground),
	}
	return bsrm.client.Delete(ctx, resource, opts...)
}

func (bsrm *buildSignResourceManager) GetBuildSignResourceByKernel(ctx context.Context, name, namespace, targetKernel string, resourceType kmmv1beta1.BuildOrSignAction, owner metav1.Object) (runtime.Object, error) {

	matchLabels := moduleKernelLabels(name, targetKernel, resourceType)
	resources, err := bsrm.getResources(ctx, namespace, matchLabels)
	if err != nil {
		return nil, fmt.Errorf("failed to get module %s, resources by kernel %s: %v", name, targetKernel, err)
	}

	// filter resources by owner, since they could have been created by the preflight
	// when checking that specific module
	moduleOwnedResources := filterResourcesByOwner(resources, owner)
	numFoundResources := len(moduleOwnedResources)
	if numFoundResources == 0 {
		return nil, buildsign.ErrNoMatchingBuildSignResource
	} else if numFoundResources > 1 {
		return nil, fmt.Errorf("expected 0 or 1 %s resources, got %d", resourceType, numFoundResources)
	}

	return &moduleOwnedResources[0], nil
}

// GetBuildSignResourceStatus returns the status of a Resource, whether the latter is in progress or not and
// whether there was an error or not
func (bsrm *buildSignResourceManager) GetBuildSignResourceStatus(obj runtime.Object) (Status, error) {
	switch obj.Status.Phase {
	case v1.PodSucceeded:
		return buildsign.StatusCompleted, nil
	case v1.PodRunning, v1.PodPending:
		return buildsign.StatusInProgress, nil
	case v1.PodFailed:
		return buildsign.StatusFailed, nil
	default:
		return "", fmt.Errorf("unknown status: %v", obj.Status)
	}
}

func (bsrm *buildSignResourceManager) IsBuildSignResourceChanged(existingObj runtime.Object, newObj runtime.Object) (bool, error) {

	existingResource, ok := existingObj.(*v1.Pod)
	if !ok {
		return errors.New("the existing resource cannot be converted to the corect resource")
	}
	newResource, ok := newObj.(*v1.Pod)
	if !ok {
		return errors.New("the new resource cannot be converted to the corect resource")
	}

	existingAnnotations := existingResource.GetAnnotations()
	newAnnotations := newResources.GetAnnotations()
	if existingAnnotations == nil {
		return false, fmt.Errorf("annotations are not present in the existing resource %s", existingResource.Name)
	}
	if existingAnnotations[constants.ResourceHashAnnotation] == newAnnotations[constants.ResourceHashAnnotation] {
		return false, nil
	}
	return true, nil
}

func (bsrm *buildSignResourceManager) GetModuleResources(ctx context.Context, modName, namespace string,
	resourceType kmmv1beta1.BuildOrSignAction, owner metav1.Object) ([]v1.Pod, error) {

	matchLabels := moduleLabels(modName, resourceType)
	resources, err := bsrm.getResources(ctx, namespace, matchLabels)
	if err != nil {
		return nil, fmt.Errorf("failed to get resources for module %s, namespace %s: %v", modName, namespace, err)
	}

	// filter resources by owner, since they could have been created by the preflight
	// when checking that specific module
	moduleOwnedResources := filterResourcesByOwner(resources, owner)
	return moduleOwnedResources, nil
}
