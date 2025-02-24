package e2e

import (
	"context"
	"github.com/kluctl/kluctl/v2/e2e/test-utils"
	"github.com/kluctl/kluctl/v2/e2e/test_project"
	"github.com/kluctl/kluctl/v2/pkg/utils/uo"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"reflect"
	"testing"
)

func createNamespace(t *testing.T, k *test_utils.EnvTestCluster, namespace string) {
	r := k.DynamicClient.Resource(v1.SchemeGroupVersion.WithResource("namespaces"))
	if _, err := r.Get(context.Background(), namespace, metav1.GetOptions{}); err == nil {
		return
	}

	var ns unstructured.Unstructured
	ns.SetName(namespace)
	_, err := r.Create(context.Background(), &ns, metav1.CreateOptions{})

	if err != nil {
		t.Fatal(err)
	}
}

func getHeadRevision(t *testing.T, p *test_project.TestProject) string {
	r := p.GetGitRepo()
	h, err := r.Head()
	if err != nil {
		t.Fatal(err)
	}
	return h.Hash().String()
}

func assertObjectExists(t *testing.T, k *test_utils.EnvTestCluster, gvr schema.GroupVersionResource, namespace string, name string) *uo.UnstructuredObject {
	x, err := k.Get(gvr, namespace, name)
	if err != nil {
		t.Fatalf("unexpected error '%v' while getting ConfigMap %s/%s", err, namespace, name)
	}
	return x
}

func assertConfigMapExists(t *testing.T, k *test_utils.EnvTestCluster, namespace string, name string) *uo.UnstructuredObject {
	return assertObjectExists(t, k, v1.SchemeGroupVersion.WithResource("configmaps"), namespace, name)
}

func assertConfigMapNotExists(t *testing.T, k *test_utils.EnvTestCluster, namespace string, name string) {
	_, err := k.Get(v1.SchemeGroupVersion.WithResource("configmaps"), namespace, name)
	if err == nil {
		t.Fatalf("expected %s/%s to not exist", namespace, name)
	}
	if !errors.IsNotFound(err) {
		t.Fatalf("unexpected error '%v' for %s/%s, expected a NotFound error", err, namespace, name)
	}
}

func assertSecretExists(t *testing.T, k *test_utils.EnvTestCluster, namespace string, name string) *uo.UnstructuredObject {
	x, err := k.Get(v1.SchemeGroupVersion.WithResource("secrets"), namespace, name)
	if err != nil {
		t.Fatalf("unexpected error '%v' while getting Secret %s/%s", err, namespace, name)
	}
	return x
}

func assertNestedFieldEquals(t *testing.T, o *uo.UnstructuredObject, expected interface{}, keys ...interface{}) {
	v, ok, err := o.GetNestedField(keys...)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("field %s not found in object", uo.KeyPath(keys).ToJsonPath())
	}
	if !reflect.DeepEqual(v, expected) {
		t.Fatalf("%v != %v", v, expected)
	}
}

func updateObject(t *testing.T, k *test_utils.EnvTestCluster, o *uo.UnstructuredObject) {
	_, err := k.DynamicClient.Resource(schema.GroupVersionResource{
		Version:  "v1",
		Resource: "configmaps",
	}).Namespace(o.GetK8sNamespace()).Update(context.Background(), o.ToUnstructured(), metav1.UpdateOptions{})
	assert.NoError(t, err)
}
