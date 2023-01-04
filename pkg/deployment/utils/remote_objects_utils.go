package utils

import (
	"context"
	"fmt"
	"github.com/kluctl/kluctl/v2/pkg/k8s"
	"github.com/kluctl/kluctl/v2/pkg/status"
	k8s2 "github.com/kluctl/kluctl/v2/pkg/types/k8s"
	"github.com/kluctl/kluctl/v2/pkg/utils"
	"github.com/kluctl/kluctl/v2/pkg/utils/uo"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sync"
)

type RemoteObjectUtils struct {
	ctx              context.Context
	dew              *DeploymentErrorsAndWarnings
	remoteObjects    map[k8s2.ObjectRef]*uo.UnstructuredObject
	remoteNamespaces map[string]*uo.UnstructuredObject
}

func NewRemoteObjectsUtil(ctx context.Context, dew *DeploymentErrorsAndWarnings) *RemoteObjectUtils {
	return &RemoteObjectUtils{
		ctx:              ctx,
		dew:              dew,
		remoteObjects:    map[k8s2.ObjectRef]*uo.UnstructuredObject{},
		remoteNamespaces: map[string]*uo.UnstructuredObject{},
	}
}

func (u *RemoteObjectUtils) getAllByLabels(k *k8s.K8sCluster, labels map[string]string) error {
	var mutex sync.Mutex
	if len(labels) == 0 {
		return nil
	}

	baseStatus := "Getting remote objects by commonLabels"
	s := status.Start(u.ctx, baseStatus)
	defer s.Failed()

	gvks := k.Resources.GetFilteredPreferredGVKs(func(ar *v1.APIResource) bool {
		return utils.FindStrInSlice(ar.Verbs, "list") != -1
	})

	g := utils.NewGoHelper(u.ctx, 0)
	for _, gvk := range gvks {
		gvk := gvk
		g.RunE(func() error {
			l, apiWarnings, err := k.ListObjects(gvk, "", labels)
			u.dew.AddApiWarnings(k8s2.ObjectRef{GVK: gvk}, apiWarnings)
			if err != nil {
				if errors2.IsNotFound(err) {
					return nil
				}
				return err
			}
			mutex.Lock()
			defer mutex.Unlock()
			for _, o := range l {
				u.remoteObjects[o.GetK8sRef()] = o
			}
			return nil
		})
	}
	g.Wait()
	if g.ErrorOrNil() == nil {
		s.Success()
	}
	return g.ErrorOrNil()
}

func (u *RemoteObjectUtils) getMissingObjects(k *k8s.K8sCluster, refs []k8s2.ObjectRef) error {
	notFoundRefsMap := make(map[k8s2.ObjectRef]bool)
	for _, ref := range refs {
		if _, ok := u.remoteObjects[ref]; !ok {
			if _, ok = notFoundRefsMap[ref]; !ok {
				notFoundRefsMap[ref] = true
			}
		}
	}

	var mutex sync.Mutex
	if len(notFoundRefsMap) == 0 {
		return nil
	}

	baseStatus := fmt.Sprintf("Getting %d additional remote objects", len(notFoundRefsMap))
	s := status.Start(u.ctx, baseStatus)
	defer s.Failed()

	g := utils.NewGoHelper(u.ctx, 0)
	for ref, _ := range notFoundRefsMap {
		ref := ref
		g.RunE(func() error {
			r, apiWarnings, err := k.GetSingleObject(ref)
			u.dew.AddApiWarnings(ref, apiWarnings)
			if err != nil {
				if errors2.IsNotFound(err) {
					return nil
				}
				return err
			}
			mutex.Lock()
			defer mutex.Unlock()
			u.remoteObjects[r.GetK8sRef()] = r
			return nil
		})
	}
	g.Wait()
	if g.ErrorOrNil() == nil {
		s.Success()
	}
	return g.ErrorOrNil()
}

func (u *RemoteObjectUtils) UpdateRemoteObjects(k *k8s.K8sCluster, labels map[string]string, refs []k8s2.ObjectRef) error {
	if k == nil {
		return nil
	}

	err := u.getAllByLabels(k, labels)
	if err != nil {
		return err
	}

	err = u.getMissingObjects(k, refs)
	if err != nil {
		return err
	}

	s := status.Start(u.ctx, "Getting namespaces")
	defer s.Failed()

	r, _, err := k.ListObjects(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Namespace",
	}, "", nil)
	if err != nil {
		return err
	}
	for _, o := range r {
		u.remoteNamespaces[o.GetK8sName()] = o
	}

	s.Success()

	return nil
}

func (u *RemoteObjectUtils) GetRemoteObject(ref k8s2.ObjectRef) *uo.UnstructuredObject {
	o, _ := u.remoteObjects[ref]
	return o
}

func (u *RemoteObjectUtils) GetRemoteNamespace(name string) *uo.UnstructuredObject {
	o, _ := u.remoteNamespaces[name]
	return o
}

func (u *RemoteObjectUtils) ForgetRemoteObject(ref k8s2.ObjectRef) {
	delete(u.remoteObjects, ref)
}

func (u *RemoteObjectUtils) GetFilteredRemoteObjects(inclusion *utils.Inclusion) []*uo.UnstructuredObject {
	var ret []*uo.UnstructuredObject

	for _, o := range u.remoteObjects {
		iv := u.getInclusionEntries(o)
		if inclusion.CheckIncluded(iv, false) {
			ret = append(ret, o)
		}
	}

	return ret
}

func (u *RemoteObjectUtils) getInclusionEntries(o *uo.UnstructuredObject) []utils.InclusionEntry {
	var iv []utils.InclusionEntry
	for _, v := range o.GetK8sLabelsWithRegex("^kluctl.io/tag-\\d+$") {
		iv = append(iv, utils.InclusionEntry{
			Type:  "tag",
			Value: v,
		})
	}

	if itemDir := o.GetK8sAnnotation("kluctl.io/kustomize_dir"); itemDir != nil {
		iv = append(iv, utils.InclusionEntry{
			Type:  "deploymentItemDir",
			Value: *itemDir,
		})
	}
	return iv
}
