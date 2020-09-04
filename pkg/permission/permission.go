/*
Copyright 2020 Sorbonne Université

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package permission

import (
	"fmt"
	"log"
	"strings"

	apps_v1alpha "edgenet/pkg/apis/apps/v1alpha"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Clientset to be synced by the custom resources
var Clientset kubernetes.Interface

// CreateClusterRoles create or update the cluster role attached to the authority
func CreateClusterRoles(authorityCopy *apps_v1alpha.Authority) error {
	// Create a cluster role to be used by authority users
	policyRule := []rbacv1.PolicyRule{{APIGroups: []string{"apps.edgenet.io"}, Resources: []string{"authorities", "totalresourcequotas"}, ResourceNames: []string{authorityCopy.GetName()}, Verbs: []string{"get"}}}
	authorityRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("authority-%s", authorityCopy.GetName())}, Rules: policyRule}
	_, err := Clientset.RbacV1().ClusterRoles().Create(authorityRole)
	if err != nil {
		log.Printf("Couldn't create authority-%s role: %s", authorityCopy.GetName(), err)
		log.Println(errors.IsAlreadyExists(err))
		if errors.IsAlreadyExists(err) {
			authorityClusterRole, err := Clientset.RbacV1().ClusterRoles().Get(authorityRole.GetName(), metav1.GetOptions{})
			if err == nil {
				authorityClusterRole.Rules = policyRule
				_, err = Clientset.RbacV1().ClusterRoles().Update(authorityClusterRole)
				if err == nil {
					log.Printf("Authority-%s cluster role updated", authorityCopy.GetName())
				}
			}
		}
	}
	return err
}

// EstablishPrivateRoleBindings generates role bindings to allow users to access their user objects and the authority to which they belong
func EstablishPrivateRoleBindings(userCopy *apps_v1alpha.User) error {
	// Put the service account dedicated to the user into the role bind subjects
	rbSubjects := []rbacv1.Subject{{Kind: "User", Name: userCopy.Spec.Email, APIGroup: "rbac.authorization.k8s.io"}}
	// This section allows the user to get user object that belongs to him. The role, which gets used by the binding object,
	// generated by the user controller when the user object created.
	roleName := fmt.Sprintf("user-%s", userCopy.GetName())
	roleRef := rbacv1.RoleRef{Kind: "Role", Name: roleName}
	roleBind := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Namespace: userCopy.GetNamespace(), Name: fmt.Sprintf("%s-%s", userCopy.GetNamespace(), roleName)},
		Subjects: rbSubjects, RoleRef: roleRef}
	_, err := Clientset.RbacV1().RoleBindings(userCopy.GetNamespace()).Create(roleBind)
	if err != nil {
		log.Printf("Couldn't create %s role binding in namespace of %s: %s, err: %s", roleName, userCopy.GetNamespace(), userCopy.GetName(), err)
		if errors.IsAlreadyExists(err) {
			userRoleBind, err := Clientset.RbacV1().RoleBindings(userCopy.GetNamespace()).Get(roleBind.GetName(), metav1.GetOptions{})
			if err == nil {
				userRoleBind.Subjects = rbSubjects
				userRoleBind.RoleRef = roleRef
				_, err = Clientset.RbacV1().RoleBindings(userCopy.GetNamespace()).Update(userRoleBind)
				if err == nil {
					log.Printf("Completed: role binding in namespace of %s: %s", userCopy.GetNamespace(), userCopy.GetName())
				}
			}
		}
	}
	// This section allows the user to get the authority object in which he/she participates. The role, which gets used by the binding object,
	// generated by the authority controller when the authority object created.
	userOwnerNamespace, _ := Clientset.CoreV1().Namespaces().Get(userCopy.GetNamespace(), metav1.GetOptions{})
	roleName = fmt.Sprintf("authority-%s", userOwnerNamespace.Labels["authority-name"])
	roleRef = rbacv1.RoleRef{Kind: "ClusterRole", Name: roleName}
	clusterRoleBind := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%s-for-authority", userCopy.GetNamespace(), userCopy.GetName())},
		Subjects: rbSubjects, RoleRef: roleRef}
	_, err = Clientset.RbacV1().ClusterRoleBindings().Create(clusterRoleBind)
	if err != nil {
		log.Printf("Couldn't create %s role binding in namespace of %s: %s", roleName, userCopy.GetNamespace(), userCopy.GetName())
		log.Println(err.Error())
	}
	return err
}

// EstablishRoleBindings generates the rolebindings according to user roles in the namespace specified
func EstablishRoleBindings(userCopy *apps_v1alpha.User, namespace string, namespaceType string) error {
	// Put the service account dedicated to the user into the role bind subjects
	rbSubjects := []rbacv1.Subject{{Kind: "User", Name: userCopy.Spec.Email, APIGroup: "rbac.authorization.k8s.io"}}
	// Roles are pre-generated by the controllers
	roleName := fmt.Sprintf("%s-%s", strings.ToLower(namespaceType), strings.ToLower(userCopy.Status.Type))
	roleRef := rbacv1.RoleRef{Kind: "ClusterRole", Name: roleName}
	roleBind := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: fmt.Sprintf("%s-%s-%s", userCopy.GetNamespace(), userCopy.GetName(), roleName)},
		Subjects: rbSubjects, RoleRef: roleRef}
	_, err := Clientset.RbacV1().RoleBindings(namespace).Create(roleBind)
	if err != nil {
		log.Printf("Couldn't create %s role binding in namespace of %s: %s - %s, err: %s", userCopy.Status.Type, namespace, userCopy.GetNamespace(), userCopy.GetName(), err)
		if errors.IsAlreadyExists(err) {
			userRoleBind, err := Clientset.RbacV1().RoleBindings(userCopy.GetNamespace()).Get(roleBind.GetName(), metav1.GetOptions{})
			if err == nil {
				userRoleBind.Subjects = rbSubjects
				userRoleBind.RoleRef = roleRef
				_, err = Clientset.RbacV1().RoleBindings(userCopy.GetNamespace()).Update(userRoleBind)
				if err == nil {
					log.Printf("Completed: %s role binding in namespace of %s: %s - %s", userCopy.Status.Type, namespace, userCopy.GetNamespace(), userCopy.GetName())
				}
			}
		}
	}
	return err
}

// CheckAuthorization returns true if the user is holder of a role
func CheckAuthorization(namespace, email, resource, resourceName string) bool {
	authorized := false
	roleBindingRaw, _ := Clientset.RbacV1().RoleBindings(namespace).List(metav1.ListOptions{})
	for _, roleBindingRow := range roleBindingRaw.Items {
		for _, subject := range roleBindingRow.Subjects {
			if subject.Kind == "User" && subject.Name == email {
				if roleBindingRow.RoleRef.Kind == "Role" {
					role, _ := Clientset.RbacV1().Roles(namespace).Get(roleBindingRow.RoleRef.Name, metav1.GetOptions{})
					for _, rule := range role.Rules {
						for _, APIGroup := range rule.APIGroups {
							if APIGroup == "apps.edgenet.io" {
								for _, ruleResource := range rule.Resources {
									if ruleResource == resource {
										if len(rule.ResourceNames) > 0 {
											for _, ruleResourceName := range rule.ResourceNames {
												if ruleResourceName == resourceName {
													authorized = true
												}
											}
										} else {
											authorized = true
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return authorized
}