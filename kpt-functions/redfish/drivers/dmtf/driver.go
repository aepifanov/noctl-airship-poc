// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dmtf

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	redfishAPI "opendev.org/airship/go-redfish/api"
	redfishClient "opendev.org/airship/go-redfish/client"

	"github.com/aodinokov/noctl-airship-poc/kpt-functions/redfish"
)

type Driver struct {
	Api redfishAPI.RedfishAPI

	AuthContextValue struct {
		Key interface{}
		Val interface{}
	}
}

func NewDriver(f *redfish.OperationFunction) (redfish.Driver, error) {
	drv := Driver{}

	if f.CredentialsSecret != nil {
		// TODO: exctract creds from f.CredentialsSecret
		drv.AuthContextValue.Key = redfishClient.ContextBasicAuth
		drv.AuthContextValue.Val = redfishClient.BasicAuth{UserName: "username", Password: "password"}
        }


	cfg := redfishClient.NewConfiguration()
	if f.Config.Spec.UserAgent != nil {
		cfg.UserAgent = *f.Config.Spec.UserAgent
	}

	// see https://github.com/golang/go/issues/26013
	// We clone the default transport to ensure when we customize the transport
	// that we are providing it sane timeouts and other defaults that we would
	// normally get when not overriding the transport
	defaultTransportCopy := http.DefaultTransport.(*http.Transport) //nolint:errcheck
	transport := defaultTransportCopy.Clone()

	if /*insecure */ false {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		}
	}

	cfg.HTTPClient = &http.Client{
		Transport: transport,
	}


	drv.Api = redfishClient.NewAPIClient(cfg).DefaultApi

	return &drv

/*
// EjectVirtualMedia ejects a virtual media device attached to a host.
func (c *Client) EjectVirtualMedia(ctx context.Context) error {
	waitForEjectMedia := func(managerID string, mediaID string) error {
		for retry := 0; retry < c.systemActionRetries; retry++ {
			vMediaMgr, httpResp, err := c.RedfishAPI.GetManagerVirtualMedia(ctx, managerID, mediaID)
			if err = ScreenRedfishError(httpResp, err); err != nil {
				return err
			}

			if *vMediaMgr.Inserted == false {
				log.Debugf("Successfully ejected virtual media.")
				return nil
			}
		}

		return ErrOperationRetriesExceeded{What: fmt.Sprintf("eject media %s", mediaID), Retries: c.systemActionRetries}
	}

	managerID, err := getManagerID(ctx, c.RedfishAPI, c.nodeID)
	if err != nil {
		return err
	}

	mediaCollection, httpResp, err := c.RedfishAPI.ListManagerVirtualMedia(ctx, managerID)
	if err = ScreenRedfishError(httpResp, err); err != nil {
		return err
	}

	// Walk all virtual media devices and eject if inserted
	for _, mediaURI := range mediaCollection.Members {
		mediaID := GetResourceIDFromURL(mediaURI.OdataId)

		vMediaMgr, httpResp, err := c.RedfishAPI.GetManagerVirtualMedia(ctx, managerID, mediaID)
		if err = ScreenRedfishError(httpResp, err); err != nil {
			return err
		}

		if *vMediaMgr.Inserted == true {
			log.Debugf("'%s' has virtual media inserted. Attempting to eject.", vMediaMgr.Name)

			var emptyBody map[string]interface{}
			_, httpResp, err = c.RedfishAPI.EjectVirtualMedia(ctx, managerID, mediaID, emptyBody)
			if err = ScreenRedfishError(httpResp, err); err != nil {
				return err
			}

			if err = waitForEjectMedia(managerID, mediaID); err != nil {
				return err
			}
		}
	}

	return nil
}

// RebootSystem power cycles a host by sending a shutdown signal followed by a power on signal.
func (c *Client) RebootSystem(ctx context.Context) error {
	log.Debugf("Rebooting node '%s': powering off.", c.nodeID)
	resetReq := redfishClient.ResetRequestBody{}

	// Send PowerOff request
	resetReq.ResetType = redfishClient.RESETTYPE_FORCE_OFF
	_, httpResp, err := c.RedfishAPI.ResetSystem(ctx, c.nodeID, resetReq)
	if err = ScreenRedfishError(httpResp, err); err != nil {
		log.Debugf("Failed to reboot node '%s': shutdown failure.", c.nodeID)
		return err
	}

	// Check that node is powered off
	if err = c.waitForPowerState(ctx, redfishClient.POWERSTATE_OFF); err != nil {
		return err
	}

	log.Debugf("Rebooting node '%s': powering on.", c.nodeID)

	// Send PowerOn request
	resetReq.ResetType = redfishClient.RESETTYPE_ON
	_, httpResp, err = c.RedfishAPI.ResetSystem(ctx, c.nodeID, resetReq)
	if err = ScreenRedfishError(httpResp, err); err != nil {
		log.Debugf("Failed to reboot node '%s': startup failure.", c.nodeID)
		return err
	}

	// Check that node is powered on and return
	return c.waitForPowerState(ctx, redfishClient.POWERSTATE_ON)
}

// SetBootSourceByType sets the boot source of the ephemeral node to one that's compatible with the boot
// source type.
func (c *Client) SetBootSourceByType(ctx context.Context) error {
	_, vMediaType, err := GetVirtualMediaID(ctx, c.RedfishAPI, c.nodeID)
	if err != nil {
		return err
	}

	log.Debugf("Setting boot device to '%s'.", vMediaType)

	// Retrieve system information, containing available boot sources
	system, _, err := c.RedfishAPI.GetSystem(ctx, c.nodeID)
	if err != nil {
		return ErrRedfishClient{Message: fmt.Sprintf("Get System[%s] failed with err: %v", c.nodeID, err)}
	}

	allowableValues := system.Boot.BootSourceOverrideTargetRedfishAllowableValues
	for _, bootSource := range allowableValues {
		if strings.EqualFold(string(bootSource), vMediaType) {
			// set boot source 
			systemReq := redfishClient.ComputerSystem{}
			systemReq.Boot.BootSourceOverrideTarget = bootSource
			_, httpResp, err := c.RedfishAPI.SetSystem(ctx, c.nodeID, systemReq)
			if err = ScreenRedfishError(httpResp, err); err != nil {
				return err
			}

			log.Debug("Successfully set boot device.")
			return nil
		}
	}

	return ErrRedfishClient{Message: fmt.Sprintf("failed to set system[%s] boot source", c.nodeID)}
}

// SetVirtualMedia injects a virtual media device to an established virtual media ID. This assumes that isoPath is
// accessible to the redfish server and virtualMedia device is either of type CD or DVD.
func (c *Client) SetVirtualMedia(ctx context.Context, isoPath string) error {
	log.Debugf("Inserting virtual media '%s'.", isoPath)
	// Eject all previously-inserted media
	if err := c.EjectVirtualMedia(ctx); err != nil {
		return err
	}

	// Retrieve the ID of a compatible media type
	vMediaID, _, err := GetVirtualMediaID(ctx, c.RedfishAPI, c.nodeID)
	if err != nil {
		return err
	}

	managerID, err := getManagerID(ctx, c.RedfishAPI, c.nodeID)
	if err != nil {
		return err
	}

	// Insert media
	vMediaReq := redfishClient.InsertMediaRequestBody{}
	vMediaReq.Image = isoPath
	vMediaReq.Inserted = true
	_, httpResp, err := c.RedfishAPI.InsertVirtualMedia(ctx, managerID, vMediaID, vMediaReq)

	if err = ScreenRedfishError(httpResp, err); err != nil {
		return err
	}

	log.Debug("Successfully set virtual media.")
	return nil
}

// SystemPowerOff shuts down a host.
func (c *Client) SystemPowerOff(ctx context.Context) error {
	resetReq := redfishClient.ResetRequestBody{}
	resetReq.ResetType = redfishClient.RESETTYPE_FORCE_OFF

	_, httpResp, err := c.RedfishAPI.ResetSystem(ctx, c.nodeID, resetReq)
	if err = ScreenRedfishError(httpResp, err); err != nil {
		return err
	}

	return c.waitForPowerState(ctx, redfishClient.POWERSTATE_OFF)
}

// SystemPowerOn powers on a host.
func (c *Client) SystemPowerOn(ctx context.Context) error {
	resetReq := redfishClient.ResetRequestBody{}
	resetReq.ResetType = redfishClient.RESETTYPE_ON

	_, httpResp, err := c.RedfishAPI.ResetSystem(ctx, c.nodeID, resetReq)
	if err = ScreenRedfishError(httpResp, err); err != nil {
		return err
	}

	return c.waitForPowerState(ctx, redfishClient.POWERSTATE_ON)
}

// SystemPowerStatus retrieves the power status of a host as a human-readable string.
func (c *Client) SystemPowerStatus(ctx context.Context) (power.Status, error) {
	computerSystem, httpResp, err := c.RedfishAPI.GetSystem(ctx, c.nodeID)
	if err = ScreenRedfishError(httpResp, err); err != nil {
		return power.StatusUnknown, err
	}

	switch computerSystem.PowerState {
	case redfishClient.POWERSTATE_ON:
		return power.StatusOn, nil
	case redfishClient.POWERSTATE_OFF:
		return power.StatusOff, nil
	case redfishClient.POWERSTATE_POWERING_ON:
		return power.StatusPoweringOn, nil
	case redfishClient.POWERSTATE_POWERING_OFF:
		return power.StatusPoweringOff, nil
	default:
		return power.StatusUnknown, nil
	}
}

// NewClient returns a client with the capability to make Redfish requests.
func NewClient(redfishURL string,
	insecure bool,
	useProxy bool,
	username string,
	password string,
	systemActionRetries int,
	systemRebootDelay int) (context.Context, *Client, error) {
	var ctx context.Context
	if username != "" && password != "" {
		ctx = context.WithValue(
			context.Background(),
			redfishClient.ContextBasicAuth,
			redfishClient.BasicAuth{UserName: username, Password: password},
		)
	} else {
		ctx = context.Background()
	}

	if redfishURL == "" {
		return ctx, nil, ErrRedfishMissingConfig{What: "Redfish URL"}
	}

	basePath, err := getBasePath(redfishURL)
	if err != nil {
		return ctx, nil, err
	}

	cfg := &redfishClient.Configuration{
		BasePath:      basePath,
		DefaultHeader: make(map[string]string),
		UserAgent:     headerUserAgent,
	}

	// see https://github.com/golang/go/issues/26013
	// We clone the default transport to ensure when we customize the transport
	// that we are providing it sane timeouts and other defaults that we would
	// normally get when not overriding the transport
	defaultTransportCopy := http.DefaultTransport.(*http.Transport) //nolint:errcheck
	transport := defaultTransportCopy.Clone()

	if insecure {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		}
	}

	if !useProxy {
		transport.Proxy = nil
	}

	cfg.HTTPClient = &http.Client{
		Transport: transport,
	}

	// Retrieve system ID from end of Redfish URL
	systemID := GetResourceIDFromURL(redfishURL)
	if len(systemID) == 0 {
		return ctx, nil, ErrRedfishMissingConfig{What: "management URL system ID"}
	}

	c := &Client{
		nodeID:              systemID,
		RedfishAPI:          redfishClient.NewAPIClient(cfg).DefaultApi,
		RedfishCFG:          cfg,
		systemActionRetries: systemActionRetries,
		systemRebootDelay:   systemRebootDelay,

		Sleep: func(d time.Duration) {
			time.Sleep(d)
		},
	}

	return ctx, c, nil
}
*/
