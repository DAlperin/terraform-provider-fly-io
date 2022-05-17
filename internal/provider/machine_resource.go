package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ tfsdk.ResourceType = flyMachineResourceType{}
var _ tfsdk.Resource = flyMachineResource{}
var _ tfsdk.ResourceWithImportState = flyMachineResource{}

//TODO: build

type flyMachineResourceType struct{}

type flyMachineResource struct {
	provider provider
}

type flyMachineResourceData struct {
}

func (mr flyMachineResourceType) GetSchema(context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: "Fly volume resource",
		Attributes:          map[string]tfsdk.Attribute{},
	}, nil
}

func (mr flyMachineResourceType) NewResource(ctx context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return flyMachineResource{
		provider: provider,
	}, diags
}

func (r flyMachineResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	var data flyMachineResourceData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

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

	resp.State.RemoveResource(ctx)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyMachineResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}
