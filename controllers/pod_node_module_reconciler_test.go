package controllers

import (
	"context"
	"fmt"

	"github.com/golang/mock/gomock"
	mock_client "github.com/kubernetes-sigs/kernel-module-management/internal/client"
	"github.com/kubernetes-sigs/kernel-module-management/internal/constants"
	"github.com/kubernetes-sigs/kernel-module-management/internal/daemonset"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("PodNodeModuleReconciler", func() {
	Describe("Reconcile", func() {
		const (
			moduleName   = "module-name"
			nodeName     = "node-name"
			podName      = "pod-name"
			podNamespace = "pod-namespace"
			nodeLabel    = "some node label"
		)

		var (
			kubeClient *mock_client.MockClient
			r          *PodNodeModuleReconciler
			mockDC     *daemonset.MockDaemonSetCreator
		)

		BeforeEach(func() {
			ctrl := gomock.NewController(GinkgoT())
			kubeClient = mock_client.NewMockClient(ctrl)
			mockDC = daemonset.NewMockDaemonSetCreator(ctrl)
			r = NewPodNodeModuleReconciler(kubeClient, mockDC)
		})

		ctx := context.Background()
		nn := types.NamespacedName{
			Namespace: podNamespace,
			Name:      podName,
		}
		req := ctrl.Request{NamespacedName: nn}

		It("should return an error if the pod is not labeled", func() {
			gomock.InOrder(
				kubeClient.EXPECT().Get(ctx, nn, gomock.AssignableToTypeOf(&v1.Pod{})),
			)

			_, err := r.Reconcile(ctx, req)
			Expect(err).To(HaveOccurred())
		})

		It("should unlabel the node when a Pod is not ready", func() {

			gomock.InOrder(
				kubeClient.EXPECT().Get(ctx, req.NamespacedName, gomock.Any()).Do(
					func(_ interface{}, _ interface{}, pod *v1.Pod, _ ...client.GetOption) {
						pod.SetLabels(map[string]string{constants.ModuleNameLabel: moduleName})
						pod.Spec.NodeName = nodeName
						pod.Name = podName
					},
				),
				mockDC.EXPECT().GetNodeLabelFromPod(gomock.Any(), moduleName).Return(nodeLabel),
				kubeClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Do(
					func(_ interface{}, _ interface{}, node *v1.Node, _ ...client.GetOption) {
						node.SetLabels(map[string]string{nodeLabel: podName})
					},
				),
				kubeClient.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()).Do(
					func(_ interface{}, node *v1.Node, p client.Patch, _ ...client.GetOption) {
						Expect(p.Type()).To(Equal(types.MergePatchType))
						Expect(p.Data(node)).To(Equal([]byte(`{"metadata":{"labels":null}}`)))
					},
				),
			)

			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should label the node when a Pod is ready", func() {

			gomock.InOrder(
				kubeClient.EXPECT().Get(ctx, req.NamespacedName, gomock.Any()).Do(
					func(_ interface{}, _ interface{}, pod *v1.Pod, _ ...client.GetOption) {
						pod.Name = podName
						pod.SetLabels(map[string]string{constants.ModuleNameLabel: moduleName})
						pod.Spec.NodeName = nodeName
						pod.Status = v1.PodStatus{
							Conditions: []v1.PodCondition{
								{
									Type:   v1.PodReady,
									Status: v1.ConditionTrue,
								},
							},
						}
					},
				),
				mockDC.EXPECT().GetNodeLabelFromPod(gomock.Any(), moduleName).Return(nodeLabel),
				kubeClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(nil),
				kubeClient.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()).Do(
					func(_ interface{}, node *v1.Node, p client.Patch, _ ...client.GetOption) {
						Expect(p.Type()).To(Equal(types.MergePatchType))
						expectedPatch := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, nodeLabel, podName)
						Expect(p.Data(node)).To(Equal([]byte(expectedPatch)))
					},
				),
			)

			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove the pod finalizer when the pod is being deleted", func() {

			var now = metav1.Now()

			gomock.InOrder(
				kubeClient.EXPECT().Get(ctx, req.NamespacedName, gomock.Any()).Do(
					func(_ interface{}, _ interface{}, pod *v1.Pod, _ ...client.GetOption) {
						pod.SetLabels(map[string]string{constants.ModuleNameLabel: moduleName})
						pod.DeletionTimestamp = &now
						pod.Finalizers = []string{constants.NodeLabelerFinalizer}
					},
				),
				mockDC.EXPECT().GetNodeLabelFromPod(gomock.Any(), moduleName).Return(nodeLabel),
				kubeClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(nil),
				kubeClient.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()),
				kubeClient.EXPECT().Patch(ctx, gomock.Any(), gomock.Any()).Do(
					func(_ interface{}, pod *v1.Pod, p client.Patch, _ ...client.GetOption) {
						Expect(p.Type()).To(Equal(types.MergePatchType))
						Expect(p.Data(pod)).To(Equal([]byte(`{"metadata":{"finalizers":null}}`)))
					},
				),
			)

			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
