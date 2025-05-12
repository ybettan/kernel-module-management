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

//const (
//	dockerfileAnnotationKey = "dockerfile"
//	dockerfileVolumeName    = "dockerfile"
//)

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
			Labels:       resourceLabels(mld.Name, mld.KernelNormalizedVersion, string(kmmv1beta1.BuildImage)),
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
			Labels:       resourceLabels(mld.Name, mld.KernelNormalizedVersion, string(kmmv1beta1.SignImage)),
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
		return errors.New("the resource cannot be converted to a pod")
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
		return errors.New("the resource cannot be converted to a pod")
	}
	opts := []client.DeleteOption{
		client.PropagationPolicy(metav1.DeletePropagationBackground),
	}
	return bsrm.client.Delete(ctx, resource, opts...)
}

//func (bsrm *buildSignResourceManager) IsPodChanged(existingPod *v1.Pod, newPod *v1.Pod) (bool, error) {
//	existingAnnotations := existingPod.GetAnnotations()
//	newAnnotations := newPod.GetAnnotations()
//	if existingAnnotations == nil {
//		return false, fmt.Errorf("annotations are not present in the existing pod %s", existingPod.Name)
//	}
//	if existingAnnotations[constants.PodHashAnnotation] == newAnnotations[constants.PodHashAnnotation] {
//		return false, nil
//	}
//	return true, nil
//}
//
//
//func (bsrm *buildSignResourceManager) GetModulePodByKernel(ctx context.Context, modName, namespace, targetKernel, podType string, owner metav1.Object) (*v1.Pod, error) {
//	matchLabels := moduleKernelLabels(modName, targetKernel, podType)
//	pods, err := bsrm.getPods(ctx, namespace, matchLabels)
//	if err != nil {
//		return nil, fmt.Errorf("failed to get module %s, pods by kernel %s: %v", modName, targetKernel, err)
//	}
//
//	// filter pods by owner, since they could have been created by the preflight
//	// when checking that specific module
//	moduleOwnedPods := filterPodsByOwner(pods, owner)
//	numFoundPods := len(moduleOwnedPods)
//	if numFoundPods == 0 {
//		return nil, ErrNoMatchingPod
//	} else if numFoundPods > 1 {
//		return nil, fmt.Errorf("expected 0 or 1 %s pod, got %d", podType, numFoundPods)
//	}
//
//	return &moduleOwnedPods[0], nil
//}
//
//func (bsrm *buildSignResourceManager) GetModulePods(ctx context.Context, modName, namespace, podType string, owner metav1.Object) ([]v1.Pod, error) {
//	matchLabels := moduleLabels(modName, podType)
//	pods, err := bsrm.getPods(ctx, namespace, matchLabels)
//	if err != nil {
//		return nil, fmt.Errorf("failed to get pods for module %s, namespace %s: %v", modName, namespace, err)
//	}
//
//	// filter pods by owner, since they could have been created by the preflight
//	// when checking that specific module
//	moduleOwnedPods := filterPodsByOwner(pods, owner)
//	return moduleOwnedPods, nil
//}
//
//
//
//// GetPodStatus returns the status of a Pod, whether the latter is in progress or not and
//// whether there was an error or not
//func (bsrm *buildSignResourceManager) GetPodStatus(pod *v1.Pod) (Status, error) {
//	switch pod.Status.Phase {
//	case v1.PodSucceeded:
//		return StatusCompleted, nil
//	case v1.PodRunning, v1.PodPending:
//		return StatusInProgress, nil
//	case v1.PodFailed:
//		return StatusFailed, nil
//	default:
//		return "", fmt.Errorf("unknown status: %v", pod.Status)
//	}
//}
//
//func (bsrm *buildSignResourceManager) getPods(ctx context.Context, namespace string, labels map[string]string) ([]v1.Pod, error) {
//	podList := v1.PodList{}
//	opts := []client.ListOption{
//		client.MatchingLabels(labels),
//		client.InNamespace(namespace),
//	}
//	if err := bsrm.client.List(ctx, &podList, opts...); err != nil {
//		return nil, fmt.Errorf("could not list pods: %v", err)
//	}
//
//	return podList.Items, nil
//}
//
//func moduleKernelLabels(moduleName, targetKernel, podType string) map[string]string {
//	labels := moduleLabels(moduleName, podType)
//	labels[constants.TargetKernelTarget] = targetKernel
//	return labels
//}
//
//func moduleLabels(moduleName, podType string) map[string]string {
//	return map[string]string{
//		constants.ModuleNameLabel: moduleName,
//		constants.PodType:         podType,
//	}
//}
//
//func filterPodsByOwner(pods []v1.Pod, owner metav1.Object) []v1.Pod {
//	ownedPods := []v1.Pod{}
//	for _, pod := range pods {
//		if metav1.IsControlledBy(&pod, owner) {
//			ownedPods = append(ownedPods, pod)
//		}
//	}
//	return ownedPods
//}
//
