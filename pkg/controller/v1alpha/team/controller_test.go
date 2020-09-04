package team

import (
	apps_v1alpha "edgenet/pkg/apis/apps/v1alpha"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStartController(t *testing.T) {
	g := TeamTestGroup{}
	g.Init()
	// Run the controller in a goroutine
	go Start(g.client, g.edgenetclient)
	// Create a team
	g.edgenetclient.AppsV1alpha().Teams(fmt.Sprintf("authority-%s", g.authorityObj.GetName())).Create(g.teamObj.DeepCopy())
	// Wait for the status update of created object
	time.Sleep(time.Millisecond * 500)
	// Get the object and check the status
	team, _ := g.edgenetclient.AppsV1alpha().Teams(g.authorityObj.GetNamespace()).Get(g.teamObj.GetName(), metav1.GetOptions{})
	if !team.Spec.Enabled {
		t.Error(errorDict["add-func"])
	}
	// Update a team
	team.Spec.Users = []apps_v1alpha.TeamUsers{
		apps_v1alpha.TeamUsers{
			Authority: g.authorityObj.GetName(),
			Username:  "user1",
		},
	}
	g.userObj.Spec.Active, g.userObj.Status.AUP = true, true
	// Creating User before updating requesting server to update internal representation of team
	g.edgenetclient.AppsV1alpha().Users(fmt.Sprintf("authority-%s", g.authorityObj.GetName())).Create(g.userObj.DeepCopy())
	// Requesting server to update internal representation of team
	g.edgenetclient.AppsV1alpha().Teams(fmt.Sprintf("authority-%s", g.authorityObj.GetName())).Update(team)
	// Check user rolebinding in team child namespace
	user, _ := g.edgenetclient.AppsV1alpha().Users(fmt.Sprintf("authority-%s", g.authorityObj.GetName())).Get("user1", metav1.GetOptions{})
	time.Sleep(time.Millisecond * 500)
	roleBindings, _ := g.client.RbacV1().RoleBindings(fmt.Sprintf("%s-team-%s", g.teamObj.GetNamespace(), g.teamObj.GetName())).Get(fmt.Sprintf("%s-%s-team-%s", user.GetNamespace(), user.GetName(), "admin"), metav1.GetOptions{})
	// Verifying server created rolebinding for new user in team's child namespace
	if roleBindings == nil {
		t.Error(errorDict["upd-func"])
	}
	// Delete a user
	// Requesting server to delete internal representation of team
	g.edgenetclient.AppsV1alpha().Teams(fmt.Sprintf("authority-%s", g.authorityObj.GetName())).Delete(g.teamObj.Name, &metav1.DeleteOptions{})
	time.Sleep(time.Millisecond * 500)
	teamChildNamespace, _ := g.client.CoreV1().Namespaces().Get(fmt.Sprintf("%s-team-%s", g.teamObj.GetNamespace(), g.teamObj.GetName()), metav1.GetOptions{})
	if teamChildNamespace != nil {
		t.Error(errorDict["del-func"])
	}
}