package buildsign

import (
	"context"
	"errors"

	"github.com/kubernetes-sigs/kernel-module-management/internal/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type Status string

const (
	StatusCompleted  Status = "completed"
	StatusCreated    Status = "created"
	StatusInProgress Status = "in progress"
	StatusFailed     Status = "failed"
)

var ErrNoMatchingBuildSignResource = errors.New("no matching pod")

//go:generate mockgen -source=buildsignresourcemanager.go -package=buildsign -destination=mock_buildsignresourcemanager.go

type BuildSignResourceManager interface {
	MakeBuildResourceTemplate(ctx context.Context, mld *api.ModuleLoaderData, owner metav1.Object,
		pushImage bool) (runtime.Object, error)
	MakeSignResourceTemplate(ctx context.Context, mld *api.ModuleLoaderData, owner metav1.Object,
		pushImage bool) (runtime.Object, error)
	CreateBuildSignResource(ctx context.Context, spec runtime.Object) error
	DeleteBuildSignResource(ctx context.Context, obj runtime.Object) error
	//GetBuildSignResourceByKernel(ctx context.Context, name, namespace, targetKernel, resourceType string,
	//	owner metav1.Object) (runtime.Object, error)
	//GetBuildSignResourceStatus(obj runtime.Object) (Status, error)
	//IsBuildSignResourceChanged(existingObj runtime.Object, newObj runtime.Object) (bool, error)
}
