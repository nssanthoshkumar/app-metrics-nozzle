package api

import (
	"testing"
	"app-usage-nozzle/domain"
	"github.com/jtgammon/go-cfclient"
	"fmt"
)


var AppDetails = make(map[string]domain.App)

func TestReverse(t *testing.T) {

	c := cfclient.Config{
		ApiAddress:        "https://api.run.haas-41.pez.pivotal.io",
		Username:          "admin",
		Password:          "cb0a40f8d6360eaed442",
		SkipSslValidation: true,
	}

	client, _ = cfclient.NewClient(&c)

	orgs, _ := client.ListOrgs()
	var o []domain.Entity // == nil
	for idx := range orgs {
		org := domain.Entity{Name:orgs[idx].Name, Guid:orgs[idx].Guid}
		o = append(o, org)
	}
	logger.Println(fmt.Sprintf("Org name %s", o))

	o = nil

	spaces, _ := client.ListSpaces()
	for idx := range spaces {
		org := domain.Entity{Name:spaces[idx].Name, Guid:spaces[idx].Guid}
		o = append(o, org)
	}
	logger.Println(fmt.Sprintf("Space name %s", o))

}
