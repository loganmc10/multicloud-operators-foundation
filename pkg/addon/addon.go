package addon

import (
	"context"
	"embed"
	"reflect"

	"github.com/stolostron/multicloud-operators-foundation/pkg/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/agent"
	"open-cluster-management.io/addon-framework/pkg/utils"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const (
	WorkManagerAddonName = "work-manager"
	// the clusterRole has been installed with the ocm-controller deployment
	clusterRoleName = "managed-cluster-workmgr"
	roleBindingName = "managed-cluster-workmgr"
)

//go:embed manifests
//go:embed manifests/chart
//go:embed manifests/chart/templates/_helpers.tpl
var ChartFS embed.FS

const ChartDir = "manifests/chart"

type GlobalValues struct {
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,"`
	ImagePullSecret string            `json:"imagePullSecret"`
	ImageOverrides  map[string]string `json:"imageOverrides,"`
	NodeSelector    map[string]string `json:"nodeSelector,"`
}

type Values struct {
	IsOCP        bool         `json:"isOCP,omitempty"`
	GlobalValues GlobalValues `json:"global,omitempty,omitempty"`
}

func NewGetValuesFunc(imageName string) addonfactory.GetValuesFunc {
	return func(cluster *clusterv1.ManagedCluster,
		addon *addonapiv1alpha1.ManagedClusterAddOn) (addonfactory.Values, error) {
		addonValues := Values{
			GlobalValues: GlobalValues{
				ImagePullPolicy: corev1.PullIfNotPresent,
				ImagePullSecret: "open-cluster-management-image-pull-credentials",
				ImageOverrides: map[string]string{
					"multicloud_manager": imageName,
				},
				NodeSelector: map[string]string{},
			},
		}
		for _, claim := range cluster.Status.ClusterClaims {
			if claim.Name == "product.open-cluster-management.io" && claim.Value == "OpenShift" {
				addonValues.IsOCP = true
			}
		}
		values, err := addonfactory.JsonStructToValues(addonValues)
		if err != nil {
			return nil, err
		}
		return values, nil
	}
}

func NewRegistrationOption(kubeClient kubernetes.Interface, addonName string) *agent.RegistrationOption {
	return &agent.RegistrationOption{
		CSRConfigurations: agent.KubeClientSignerConfigurations(addonName, addonName),
		CSRApproveCheck:   utils.DefaultCSRApprover(addonName),
		PermissionConfig: func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
			return createOrUpdateRoleBinding(kubeClient, addonName, cluster.Name)
		},
	}
}

// createOrUpdateRoleBinding create or update a role binding for a given cluster
func createOrUpdateRoleBinding(kubeClient kubernetes.Interface, addonName, clusterName string) error {
	groups := agent.DefaultGroups(clusterName, addonName)
	acmRoleBinding := helpers.NewRoleBindingForClusterRole(clusterRoleName, clusterName).Groups(groups[0]).BindingOrDie()

	// role and rolebinding have the same name
	binding, err := kubeClient.RbacV1().RoleBindings(clusterName).Get(context.TODO(), roleBindingName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = kubeClient.RbacV1().RoleBindings(clusterName).Create(context.TODO(), &acmRoleBinding, metav1.CreateOptions{})
		}
		return err
	}

	needUpdate := false
	if !reflect.DeepEqual(acmRoleBinding.RoleRef, binding.RoleRef) {
		needUpdate = true
		binding.RoleRef = acmRoleBinding.RoleRef
	}
	if !reflect.DeepEqual(acmRoleBinding.Subjects, binding.Subjects) {
		needUpdate = true
		binding.Subjects = acmRoleBinding.Subjects
	}
	if needUpdate {
		_, err = kubeClient.RbacV1().RoleBindings(clusterName).Update(context.TODO(), binding, metav1.UpdateOptions{})
		return err
	}

	return nil
}