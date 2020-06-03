// licensed Materials - Property of IBM
// 5737-E67
// (C) Copyright IBM Corporation 2016, 2019 All Rights Reserved
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package clusterrbac

import (
	actionv1beta1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/action/v1beta1"
	clusterv1beta1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/cluster/v1beta1"
	spokeviewv1beta1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/view/v1beta1"
	proxyserverv1beta1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/proxyserver/apis/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// BuildClusterRoleRules builds the clusteroles
func buildRoleRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		NewRule("create", "get").Groups(proxyserverv1beta1.GroupName).Resources("clusterstatuses/aggregator").RuleOrDie(),
		NewRule("get", "list", "watch").Groups(clusterv1beta1.GroupName).Resources("clusterinfos").RuleOrDie(),
		NewRule("update", "patch").Groups(clusterv1beta1.GroupName).Resources("clusterinfos/status").RuleOrDie(),
		NewRule("get", "list", "watch").Groups(actionv1beta1.GroupName).Resources("clusteractions").RuleOrDie(),
		NewRule("update", "patch").Groups(actionv1beta1.GroupName).Resources("clusteractions/status").RuleOrDie(),
		NewRule("get", "list", "watch").Groups(spokeviewv1beta1.GroupName).Resources("spokeviews").RuleOrDie(),
		NewRule("update", "patch").Groups(spokeviewv1beta1.GroupName).Resources("spokeviews/status").RuleOrDie(),

		// for deployables
		NewRule("get", "list", "watch").Groups("apps.open-cluster-management.io").Resources("deployables").RuleOrDie(),
		NewRule("patch", "update").Groups("apps.open-cluster-management.io").Resources("deployables/status").RuleOrDie(),

		NewRule("create", "update", "patch").Groups("").Resources("events").RuleOrDie(),
		NewRule("create", "update", "delete").Groups("").Resources("secrets").RuleOrDie(),
		NewRule("create", "get", "list", "watch").Groups("").Resources("secrets").RuleOrDie(),
	}
}