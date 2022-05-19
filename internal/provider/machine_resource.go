package provider

import (
	"bytes"
	"context"
	"dov.dev/fly/fly-provider/internal/utils"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"io/ioutil"
	"net/http"
	"time"
)

var _ tfsdk.ResourceType = flyMachineResourceType{}
var _ tfsdk.Resource = flyMachineResource{}
var _ tfsdk.ResourceWithImportState = flyMachineResource{}

//TODO: build

type flyMachineResourceType struct {
	Token string
}

type flyMachineResource struct {
	provider provider
	http     http.Client
}

type flyMachineResourceData struct {
	Name     types.String `tfsdk:"name"`
	Region   types.String `tfsdk:"region"`
	Id       types.String `tfsdk:"id"`
	App      types.String `tfsdk:"app"`
	Image    types.String `tfsdk:"image"`
	Cpus     types.Int64  `tfsdk:"cpus"`
	MemoryMb types.Int64  `tfsdk:"memorymb"`
	CpuType  types.String `tfsdk:"cputype"`
}

type GuestConfig struct {
	Cpus     int    `json:"cpus,omitempty"`
	MemoryMb int    `json:"memory_mb,omitempty"`
	CpuType  string `json:"cpu_type,omitempty"`
}

type MachineConfig struct {
	Image string `json:"image"`
}

type CreateMachineRequest struct {
	Name   string        `json:"name"`
	Config MachineConfig `json:"config"`
	Guest  *GuestConfig  `json:"guest,omitempty"`
}

type CreateMachineResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	State      string `json:"state"`
	Region     string `json:"region"`
	InstanceID string `json:"instance_id"`
	PrivateIP  string `json:"private_ip"`
	Config     struct {
		Env  interface{} `json:"env"`
		Init struct {
			Exec       interface{} `json:"exec"`
			Entrypoint interface{} `json:"entrypoint"`
			Cmd        interface{} `json:"cmd"`
			Tty        bool        `json:"tty"`
		} `json:"init"`
		Image    string      `json:"image"`
		Metadata interface{} `json:"metadata"`
		Restart  struct {
			Policy string `json:"policy"`
		} `json:"restart"`
		Guest struct {
			CPUKind  string `json:"cpu_kind"`
			Cpus     int    `json:"cpus"`
			MemoryMb int    `json:"memory_mb"`
		} `json:"guest"`
	} `json:"config"`
	ImageRef struct {
		Registry   string `json:"registry"`
		Repository string `json:"repository"`
		Tag        string `json:"tag"`
		Digest     string `json:"digest"`
		Labels     struct {
			Maintainer string `json:"maintainer"`
		} `json:"labels"`
	} `json:"image_ref"`
	CreatedAt time.Time `json:"created_at"`
}

func (mr flyMachineResourceType) GetSchema(context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "Fly machine resource",
		Attributes: map[string]tfsdk.Attribute{
			"name": {
				MarkdownDescription: "machine name",
				Required:            true,
				Type:                types.StringType,
			},
			"region": {
				MarkdownDescription: "machine region",
				Required:            true,
				Type:                types.StringType,
			},
			"id": {
				MarkdownDescription: "machine id",
				Computed:            true,
				Type:                types.StringType,
			},
			"app": {
				MarkdownDescription: "fly app",
				Required:            true,
				Type:                types.StringType,
			},
			"image": {
				MarkdownDescription: "docker image",
				Required:            true,
				Type:                types.StringType,
			},
			"cputype": {
				MarkdownDescription: "cpu type",
				Computed:            true,
				Optional:            true,
				Type:                types.StringType,
			},
			"cpus": {
				MarkdownDescription: "cpu count",
				Computed:            true,
				Optional:            true,
				Type:                types.Int64Type,
			},
			"memorymb": {
				MarkdownDescription: "memory mb",
				Computed:            true,
				Optional:            true,
				Type:                types.Int64Type,
			},
		},
	}, nil
}

func (mr flyMachineResourceType) NewResource(ctx context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	h := http.Client{Timeout: 60 * time.Second, Transport: &utils.Transport{UnderlyingTransport: http.DefaultTransport, Token: mr.Token, Ctx: ctx}}
	tflog.Info(ctx, "creating resource")
	return flyMachineResource{
		provider: provider,
		http:     h,
	}, diags
}

func (mr flyMachineResource) ValidateOpenTunnel() (bool, error) {
	//HACK: This is not a good way to do this, but I'm tired. Future me, please fix this.
	response, err := mr.http.Get("http://127.0.0.1:4280/bogus")
	if err != nil {
		return false, err
	}
	if response.Status == "404 Not Found" {
		return true, nil
	} else {
		return false, errors.New("unexpected in ValidateOpenTunnel. File an issue")
	}
}

func (mr flyMachineResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	var data flyMachineResourceData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	tflog.Info(ctx, "creating resource before")

	_, err := mr.ValidateOpenTunnel()
	if err != nil {
		resp.Diagnostics.AddError("fly wireguard tunnel must be open", err.Error())
		return
	}

	tflog.Info(ctx, "creating resource")

	createReq := CreateMachineRequest{
		Name:   data.Name.Value,
		Config: MachineConfig{Image: data.Image.Value},
	}
	if !data.Cpus.Unknown {
		createReq.Guest.Cpus = int(data.Cpus.Value)
	}
	if !data.CpuType.Unknown {
		createReq.Guest.CpuType = data.CpuType.Value
	}
	if !data.MemoryMb.Unknown {
		createReq.Guest.MemoryMb = int(data.MemoryMb.Value)
	}
	body, _ := json.Marshal(createReq)
	var prettyJSON bytes.Buffer
	_ = json.Indent(&prettyJSON, body, "", "\t")
	tflog.Info(ctx, prettyJSON.String())
	tflog.Info(ctx, fmt.Sprintf("http://127.0.0.1:4280/v1/apps/%s/machines", data.App.Value))
	createResponse, err := mr.http.Post(fmt.Sprintf("http://127.0.0.1:4280/v1/apps/%s/machines", data.App.Value), "application/json", bytes.NewBuffer(body))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create machine", err.Error())
		return
	}

	tflog.Info(ctx, fmt.Sprintf("%+v", createResponse))
	defer createResponse.Body.Close()

	var newMachine CreateMachineResponse
	if createResponse.StatusCode == http.StatusCreated || createResponse.StatusCode == http.StatusOK {
		err := json.NewDecoder(createResponse.Body).Decode(&newMachine)
		if err != nil {
			resp.Diagnostics.AddError("Failed to decode response machine", err.Error())
			return
		}
	} else {
		mp := make(map[string]interface{})
		_ = json.NewDecoder(createResponse.Body).Decode(&mp)
		resp.Diagnostics.AddError("Request failed", fmt.Sprintf("%s, %s, %+v", createResponse.Status, createResponse.Request.RequestURI, mp))
		return
	}

	tflog.Info(ctx, fmt.Sprintf("%+v", newMachine))

	data = flyMachineResourceData{
		Name:     types.String{Value: newMachine.Name},
		Region:   types.String{Value: newMachine.Region},
		Id:       types.String{Value: newMachine.ID},
		App:      types.String{Value: data.App.Value},
		Image:    types.String{Value: newMachine.Config.Image},
		Cpus:     types.Int64{Value: int64(newMachine.Config.Guest.Cpus)},
		MemoryMb: types.Int64{Value: int64(newMachine.Config.Guest.MemoryMb)},
		CpuType:  types.String{Value: newMachine.Config.Guest.CPUKind},
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (mr flyMachineResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	var data flyMachineResourceData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	_, err := mr.ValidateOpenTunnel()
	if err != nil {
		resp.Diagnostics.AddError("fly wireguard tunnel must be open", err.Error())
	}

	readResponse, err := mr.http.Get(fmt.Sprintf("http://127.0.0.1:4280/v1/apps/%s/machines/%s", data.App.Value, data.Id.Value))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create machine", err.Error())
		return
	}
	defer readResponse.Body.Close()

	var machine CreateMachineResponse
	if readResponse.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(readResponse.Body)
		if err != nil {
			resp.Diagnostics.AddError("Failed to read machine creation response", err.Error())
			return
		}
		err = json.Unmarshal(body, &machine)
		if err != nil {
			resp.Diagnostics.AddError("Failed to read machine creation response", err.Error())
			return
		}
	} else {
		mp := make(map[string]interface{})
		_ = json.NewDecoder(readResponse.Body).Decode(&mp)
		resp.Diagnostics.AddError("Machine read request failed", fmt.Sprintf("%s, %s, %+v", readResponse.Status, readResponse.Request.RequestURI, mp))
		return
	}
	//TODO use blocking /wait api call

	data = flyMachineResourceData{
		Name:     types.String{Value: machine.Name},
		Id:       types.String{Value: machine.ID},
		Region:   types.String{Value: machine.Region},
		App:      types.String{Value: data.App.Value},
		Image:    types.String{Value: machine.Config.Image},
		Cpus:     types.Int64{Value: int64(machine.Config.Guest.Cpus)},
		MemoryMb: types.Int64{Value: int64(machine.Config.Guest.MemoryMb)},
		CpuType:  types.String{Value: machine.Config.Guest.CPUKind},
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (mr flyMachineResource) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	resp.Diagnostics.AddError("Machine update not available", "Unimplemented")
	return

	//var plan flyMachineResourceData
	//
	//diags := req.Plan.Get(ctx, &plan)
	//resp.Diagnostics.Append(diags...)
	//
	//if resp.Diagnostics.HasError() {
	//	return
	//}
	//
	//var state flyMachineResourceData
	//diags = resp.State.Get(ctx, &state)
	//resp.Diagnostics.Append(diags...)
	//
	//if !state.Name.Unknown && plan.Name.Value != state.Name.Value {
	//	resp.Diagnostics.AddError("Can't mutate name of existing machine", "Can't swith name "+state.Name.Value+" to "+plan.Name.Value)
	//}
	//if !state.Name.Unknown && plan.Name.Value != state.Name.Value {
	//	resp.Diagnostics.AddError("Can't mutate name of existing machine", "Can't swith name "+state.Name.Value+" to "+plan.Name.Value)
	//}
	//
	//resp.State.Set(ctx, state)
	//if resp.Diagnostics.HasError() {
	//	return
	//}
}

func (mr flyMachineResource) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var data flyMachineResourceData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	_, err := mr.ValidateOpenTunnel()
	if err != nil {
		resp.Diagnostics.AddError("fly wireguard tunnel must be open", err.Error())
	}

	maxRetries := 10
	deleted := false

	for i := 0; i < maxRetries; i++ {
		readResponse, err := mr.http.Get(fmt.Sprintf("http://127.0.0.1:4280/v1/apps/%s/machines/%s", data.App.Value, data.Id.Value))
		if err != nil {
			resp.Diagnostics.AddError("Failed to get machine", err.Error())
			return
		}
		if readResponse.StatusCode == 200 {
			var machine CreateMachineResponse
			body, err := ioutil.ReadAll(readResponse.Body)
			if err != nil {
				resp.Diagnostics.AddError("Failed to read machine response", err.Error())
				return
			}
			err = json.Unmarshal(body, &machine)
			if err != nil {
				resp.Diagnostics.AddError("Failed to read machine response", err.Error())
				return
			}
			if machine.State == "started" {
				_, _ = mr.http.Post(fmt.Sprintf("http://127.0.0.1:4280/v1/apps/%s/machines/%s/stop", data.App.Value, data.Id.Value), "application/json", nil)
			}
			if machine.State == "stopping" || machine.State == "destroying" {
				time.Sleep(5 * time.Second)
			}
			if machine.State == "stopped" {
				req, err := http.NewRequest("DELETE", fmt.Sprintf("http://127.0.0.1:4280/v1/apps/%s/machines/%s", data.App.Value, data.Id.Value), nil)
				if err != nil {
					resp.Diagnostics.AddError("Failed to create deletion request", err.Error())
					return
				}
				_, _ = mr.http.Do(req)
			}
			if machine.State == "destroyed" {
				deleted = true
				break
			}
		}
	}

	if !deleted {
		resp.Diagnostics.AddError("Machine delete failed", "max retries exceeded")
		return
	}

	resp.State.RemoveResource(ctx)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (mr flyMachineResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}
