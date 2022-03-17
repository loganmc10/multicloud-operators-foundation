package addon

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakekube "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clienttesting "k8s.io/client-go/testing"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/agent"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/yaml"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = routev1.Install(scheme)
}

func newCluster(name string, ocp bool) *clusterv1.ManagedCluster {
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if ocp {
		cluster.Status = clusterv1.ManagedClusterStatus{
			ClusterClaims: []clusterv1.ManagedClusterClaim{
				{
					Name:  "product.open-cluster-management.io",
					Value: "OpenShift",
				},
			},
		}
	}
	return cluster
}

func newAddon(name, cluster, installNamespace string, annotationValues string) *addonapiv1alpha1.ManagedClusterAddOn {
	addon := &addonapiv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cluster,
		},
		Spec: *&addonapiv1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: installNamespace,
		},
	}
	if annotationValues != "" {
		addon.SetAnnotations(map[string]string{"addon.open-cluster-management.io/values": annotationValues})
	}
	return addon
}

func newAgentAddon(t *testing.T) agent.AgentAddon {
	registrationOption := NewRegistrationOption(nil, WorkManagerAddonName)
	getValuesFunc := NewGetValuesFunc("quay.io/stolostron/multicloud-manager:2.5.0")
	agentAddon, err := addonfactory.NewAgentAddonFactory(WorkManagerAddonName, ChartFS, ChartDir).
		WithScheme(scheme).
		WithGetValuesFuncs(getValuesFunc, addonfactory.GetValuesFromAddonAnnotation).
		WithAgentRegistrationOption(registrationOption).
		WithInstallStrategy(agent.InstallAllStrategy("open-cluster-management-agent-addon")).
		BuildHelmAgentAddon()
	if err != nil {
		t.Fatalf("failed to build agent %v", err)

	}
	return agentAddon
}

func output(t *testing.T, name string, objects ...runtime.Object) {
	tmpDir, err := os.MkdirTemp("./", name)
	if err != nil {
		t.Fatalf("failed to create temp %v", err)
	}

	for i, o := range objects {
		data, err := yaml.Marshal(o)
		if err != nil {
			t.Fatalf("failed yaml marshal %v", err)
		}

		err = ioutil.WriteFile(fmt.Sprintf("%v/%v-%v.yaml", tmpDir, i, o.GetObjectKind().GroupVersionKind().Kind), data, 0644)
		if err != nil {
			t.Fatalf("failed to Marshal object.%v", err)
		}

	}
}

func TestManifest(t *testing.T) {
	tests := []struct {
		name              string
		cluster           *clusterv1.ManagedCluster
		addon             *addonapiv1alpha1.ManagedClusterAddOn
		expectedNamespace string
		expectedImage     string
		expectServiceType v1.ServiceType
		expectedCount     int
	}{
		{
			name:              "is OCP",
			cluster:           newCluster("cluster1", true),
			addon:             newAddon("work-manager", "cluster1", "", `{"global":{"imageOverrides":{"multicloud_manager":"quay.io/test/multicloud_manager:test"}}}`),
			expectedNamespace: "open-cluster-management-agent-addon",
			expectedImage:     "quay.io/test/multicloud_manager:test",
			expectedCount:     6,
		},
		{
			name:              "is OCP but hub cluster",
			cluster:           newCluster("local-cluster", true),
			addon:             newAddon("work-manager", "cluster1", "", `{"global":{"nodeSelector":{"node-role.kubernetes.io/infra":""},"imageOverrides":{"multicloud_manager":"quay.io/test/multicloud_manager:test"}}}`),
			expectedNamespace: "open-cluster-management-agent-addon",
			expectedImage:     "quay.io/test/multicloud_manager:test",
			expectedCount:     5,
		},
		{
			name:              "is not OCP",
			cluster:           newCluster("cluster1", false),
			addon:             newAddon("work-manager", "cluster1", "test", ""),
			expectedNamespace: "test",
			expectedImage:     "quay.io/stolostron/multicloud-manager:2.5.0",
			expectServiceType: v1.ServiceTypeLoadBalancer,
			expectedCount:     5,
		},
	}

	agentAddon := newAgentAddon(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objects, err := agentAddon.Manifests(test.cluster, test.addon)
			if err != nil {
				t.Errorf("failed to get manifests with error %v", err)
			}

			if len(objects) != test.expectedCount {
				t.Errorf("expected objects number is %d, got %d", test.expectedCount, len(objects))
			}

			for _, o := range objects {
				switch object := o.(type) {
				case *appsv1.Deployment:
					if object.Namespace != test.expectedNamespace {
						t.Errorf("expected namespace is %s, but got %s", test.expectedNamespace, object.Namespace)
					}
					if object.Spec.Template.Spec.Containers[0].Image != test.expectedImage {
						t.Errorf("expected image is %s, but got %s", test.expectedImage, object.Spec.Template.Spec.Containers[0].Image)
					}
				case *v1.Service:
					if object.Spec.Type != test.expectServiceType {
						t.Errorf("expected service type is %s, but got %s ", test.expectServiceType, object.Spec.Type)
					}
				}
			}

			// output is for debug
			// output(t, test.name, objects...)
		})
	}
}

func TestCreateOrUpdateRoleBinding(t *testing.T) {
	tests := []struct {
		name            string
		initObjects     []runtime.Object
		clusterName     string
		validateActions func(t *testing.T, actions []clienttesting.Action)
	}{
		{
			name:        "create a new rolebinding",
			initObjects: []runtime.Object{},
			clusterName: "cluster1",
			validateActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 2 {
					t.Errorf("expecte 2 actions, but got %v", actions)
				}

				createAction := actions[1].(clienttesting.CreateActionImpl)
				createObject := createAction.Object.(*rbacv1.RoleBinding)

				groups := agent.DefaultGroups("cluster1", "work-manager")

				if createObject.Subjects[0].Name != groups[0] {
					t.Errorf("Expected group name is %s, but got %s", groups[0], createObject.Subjects[0].Name)
				}
			},
		},
		{
			name: "no update",
			initObjects: []runtime.Object{
				&rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterRoleName,
						Namespace: "cluster1",
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:     rbacv1.GroupKind,
							APIGroup: "rbac.authorization.k8s.io",
							Name:     agent.DefaultGroups("cluster1", "work-manager")[0],
						},
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "ClusterRole",
						Name:     clusterRoleName,
					},
				},
			},
			clusterName: "cluster1",
			validateActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 1 {
					t.Errorf("expecte 0 actions, but got %v", actions)
				}
			},
		},
		{
			name: "update rolebinding",
			initObjects: []runtime.Object{
				&rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterRoleName,
						Namespace: "cluster1",
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:     rbacv1.GroupKind,
							APIGroup: "rbac.authorization.k8s.io",
							Name:     "test",
						},
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "ClusterRole",
						Name:     clusterRoleName,
					},
				},
			},
			clusterName: "cluster1",
			validateActions: func(t *testing.T, actions []clienttesting.Action) {
				if len(actions) != 2 {
					t.Errorf("expecte 2 actions, but got %v", actions)
				}

				updateAction := actions[1].(clienttesting.UpdateActionImpl)
				updateObject := updateAction.Object.(*rbacv1.RoleBinding)

				groups := agent.DefaultGroups("cluster1", "work-manager")

				if updateObject.Subjects[0].Name != groups[0] {
					t.Errorf("Expected group name is %s, but got %s", groups[0], updateObject.Subjects[0].Name)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := fakekube.NewSimpleClientset(test.initObjects...)
			err := createOrUpdateRoleBinding(kubeClient, "work-manager", test.clusterName)
			if err != nil {
				t.Errorf("createOrUpdateRoleBinding expected no error, but got %v", err)
			}

			test.validateActions(t, kubeClient.Actions())
		})
	}
}
