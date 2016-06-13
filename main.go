/*
Copyright 2016 Pivotal

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

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/CrowdSurge/banner"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/boltdb/bolt"
	"github.com/cloudfoundry-community/firehose-to-syslog/firehose"
	jgClient "github.com/jtgammon/go-cfclient"

	"app-usage-nozzle/service"
	"app-usage-nozzle/usageevents"
	"app-usage-nozzle/domain"
	"app-usage-nozzle/api"
	"github.com/cloudfoundry-community/firehose-to-syslog/caching"
)

var (
	debug = kingpin.Flag("debug", "Enable debug mode. This disables forwarding to syslog").Default("false").OverrideDefaultFromEnvar("DEBUG").Bool()
	apiEndpoint = kingpin.Flag("api-endpoint", "Api endpoint address. For bosh-lite installation of CF: https://api.10.244.0.34.xip.io").OverrideDefaultFromEnvar("API_ENDPOINT").Required().String()
	dopplerEndpoint = kingpin.Flag("doppler-endpoint", "Overwrite default doppler endpoint return by /v2/info").OverrideDefaultFromEnvar("DOPPLER_ENDPOINT").String()
	subscriptionID = kingpin.Flag("subscription-id", "Id for the subscription.").Default("firehose").OverrideDefaultFromEnvar("FIREHOSE_SUBSCRIPTION_ID").String()
	user = kingpin.Flag("user", "Admin user.").Default("admin").OverrideDefaultFromEnvar("FIREHOSE_USER").String()
	password = kingpin.Flag("password", "Admin password.").Default("admin").OverrideDefaultFromEnvar("FIREHOSE_PASSWORD").String()
	skipSSLValidation = kingpin.Flag("skip-ssl-validation", "Please don't").Default("false").OverrideDefaultFromEnvar("SKIP_SSL_VALIDATION").Bool()
	cloudControllerRefreshMin = kingpin.Flag("cloud-control-refresh-min", "Cloud Controller API call interval in minutes").OverrideDefaultFromEnvar("CLOUD_CONTROLLER_REFRESH_MIN").Int()
	boltDatabasePath = kingpin.Flag("boltdb-path", "Bolt Database path ").Default("my.db").OverrideDefaultFromEnvar("BOLTDB_PATH").String()
	tickerTime = kingpin.Flag("cc-pull-time", "CloudController Polling time in sec").Default("60s").OverrideDefaultFromEnvar("CF_PULL_TIME").Duration()
)

const (
	version = "0.0.1"
)
var logger = log.New(os.Stdout, "", 0)
func main() {

	banner.Print("usage nozzle")
	port := os.Getenv("PORT")
	if len(port) == 0 {
		port = "3000"
	}

	// Start web server
	go func() {
		server := service.NewServer()
		server.Run(":" + port)
	}()

	kingpin.Version(version)
	kingpin.Parse()

	logger.Println(fmt.Sprintf("Starting app-usage-nozzle %s ", version))

	c := jgClient.Config{
		ApiAddress:        *apiEndpoint,
		Username:          *user,
		Password:          *password,
		SkipSslValidation: *skipSSLValidation,
	}
	cfClient, _ := jgClient.NewClient(&c)

	if len(*dopplerEndpoint) > 0 {
		cfClient.Endpoint.DopplerEndpoint = *dopplerEndpoint
	}
	logger.Println(fmt.Sprintf("Using %s as doppler endpoint", cfClient.Endpoint.DopplerEndpoint))

	//Use bolt for in-memory  - file caching
	db, err := bolt.Open(*boltDatabasePath, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		logger.Fatal("Error opening bolt db: ", err)
		os.Exit(1)

	}
	defer db.Close()

	caching.SetCfClient(cfClient)
	caching.SetAppDb(db)
	caching.CreateBucket()

	//Let's Update the database the first time
	reloadApps()
	reloadEvnDetails()
	lastReloaded := time.Now()
	fmt.Println("Reloaded first time:", lastReloaded)

	// Ticker Polling the CC every X sec
	ccPolling := time.NewTicker(*tickerTime)

	go func() {
		for range ccPolling.C {
			now :=time.Now()

			//timeToReload := now.Add(-cloudControllerRefreshMin * time.Minute)
			//todo figure out why this ^^^^ does not work
			timeToReload := now.Add(-5 * time.Minute)
			if timeToReload.After(lastReloaded) {
				fmt.Println("Reloaded:", timeToReload)
				reloadApps()
				reloadEvnDetails()
				lastReloaded = now
			}
		}
	}()

	token, _ := cfClient.GetToken()

	firehose := firehose.CreateFirehoseChan(cfClient.Endpoint.DopplerEndpoint, token, *subscriptionID, *skipSSLValidation)
	if firehose != nil {
		usageevents.ProcessEvents(firehose)
		logger.Println("Firehose Subscription Succesfull! Routing events...")
	} else {
		logger.Fatal("Failed connecting to Firehose...Please check settings and try again!")
	}
}

func reloadEvnDetails() {
	usageevents.Orgs = api.OrgsDetailsFromCloudController()
	usageevents.Spaces = api.SpacesDetailsFromCloudController()
}

func reloadApps() {
	logger.Println("Start filling app/space/org cache.")
	apps := caching.GetAllApp()
	for idx := range apps {
		org := apps[idx].OrgName
		space := apps[idx].SpaceName
		app := apps[idx].Name
		key := usageevents.GetMapKeyFromAppData(org, space, app)

		appId := apps[idx].Guid
		name := apps[idx].Name

		appDetail := domain.App{GUID:appId, Name:name}
		api.AnnotateWithCloudControllerData(&appDetail)
		usageevents.AppDetails[key] = appDetail
		logger.Println(fmt.Sprintf("Registered [%s]", key))
	}

	logger.Println(fmt.Sprintf("Done filling cache! Found [%d] Apps", len(apps)))

}
