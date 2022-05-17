package provider

import (
	"context"
	"dov.dev/fly/fly-provider/graphql"
	"dov.dev/fly/fly-provider/internal/provider/modifiers"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

var _ tfsdk.ResourceType = flyMachineResourceType{}
var _ tfsdk.Resource = flyMachineResource{}
var _ tfsdk.ResourceWithImportState = flyMachineResource{}

type flyMachineResourceType struct{}

type flyMachineResource struct {
	provider provider
}

type flyMachineResourceData struct {
	Id      types.String `tfsdk:"id"`
	Appid   types.String `tfsdk:"app"`
	Region  types.String `tfsdk:"region"`
	Address types.String `tfsdk:"address"`
	Type    types.String `tfsdk:"type"`
}

func (t flyMachineResourceType) GetSchema(context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "Fly volume resource",
		Attributes: map[string]tfsdk.Attribute{
			"address": {
				MarkdownDescription: "ID of volume",
				Type:                types.StringType,
				Computed:            true,
			},
			"app": {
				MarkdownDescription: "Name of app to attach",
				Required:            true,
				Type:                types.StringType,
			},
			"id": {
				MarkdownDescription: "ID of address",
				Computed:            true,
				Type:                types.StringType,
			},
			"type": {
				MarkdownDescription: "v4 or v6",
				Type:                types.StringType,
				Required:            true,
			},
			"region": {
				MarkdownDescription: "region",
				Type:                types.StringType,
				Computed:            true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					modifiers.StringDefault("global"),
				},
			},
		},
	}, nil
}

func (t flyMachineResourceType) NewResource(ctx context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return flyMachineResource{
		provider: provider,
	}, diags
}

func (r flyMachineResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	var data flyMachineResourceData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	q, err := graphql.AllocateIpAddress(context.Background(), *r.provider.client, data.Appid.Value, data.Region.Value, graphql.IPAddressType(data.Type.Value))
	tflog.Info(ctx, fmt.Sprintf("query res in create ip: %+v", q))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create ip addr", err.Error())
	}

	data = flyMachineResourceData{
		Id:      types.String{Value: q.AllocateIpAddress.IpAddress.Id},
		Appid:   types.String{Value: data.Appid.Value},
		Region:  types.String{Value: q.AllocateIpAddress.IpAddress.Region},
		Type:    types.String{Value: string(q.AllocateIpAddress.IpAddress.Type)},
		Address: types.String{Value: q.AllocateIpAddress.IpAddress.Address},
	}

	tflog.Info(ctx, fmt.Sprintf("%+v", data))

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyMachineResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	var data flyMachineResourceData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	addr := data.Address.Value
	app := data.Appid.Value

	//tflog.Info(ctx, fmt.Sprintf("%s, %s", app, addr))
	query, err := graphql.IpAddressQuery(context.Background(), *r.provider.client, app, addr)
	tflog.Info(ctx, fmt.Sprintf("Query res: for %s %s %+v", app, addr, query))
	var errList gqlerror.List
	if errors.As(err, &errList) {
		for _, err := range errList {
			tflog.Info(ctx, "IN HERE")
			if err.Message == "Could not resolve " {
				return
			}
			resp.Diagnostics.AddError(err.Message, err.Path.String())
		}
	} else if err != nil {
		resp.Diagnostics.AddError("Read: query failed", err.Error())
	}

	data = flyMachineResourceData{
		Id:      types.String{Value: query.App.IpAddress.Id},
		Appid:   types.String{Value: data.Appid.Value},
		Region:  types.String{Value: query.App.IpAddress.Region},
		Type:    types.String{Value: string(query.App.IpAddress.Type)},
		Address: types.String{Value: query.App.IpAddress.Address},
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyMachineResource) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	resp.Diagnostics.AddError("The fly api does not allow updating ips once created", "Try deleting and then recreating the ip with new options")
	return
}

func (r flyMachineResource) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var data flyMachineResourceData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if !data.Id.Unknown && !data.Id.Null && data.Id.Value != "" {
		_, err := graphql.ReleaseIpAddress(context.Background(), *r.provider.client, data.Id.Value)
		if err != nil {
			resp.Diagnostics.AddError("Release ip failed", err.Error())
		}
	}

	resp.State.RemoveResource(ctx)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyMachineResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}
