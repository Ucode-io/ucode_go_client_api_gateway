package v1

import (
	"context"
	"errors"
	"ucode/ucode_go_client_api_gateway/api/models"
	"ucode/ucode_go_client_api_gateway/genproto/auth_service"
	pb "ucode/ucode_go_client_api_gateway/genproto/company_service"
	"ucode/ucode_go_client_api_gateway/pkg/util"

	"ucode/ucode_go_client_api_gateway/api/status_http"

	"github.com/gin-gonic/gin"
)

const (
	FUNCTION = "FUNCTION"
)

// InvokeFunctionByPath godoc
// @Security ApiKeyAuth
// @Param function-path path string true "function-path"
// @ID invoke_function_by_path
// @Router /v1/invoke_function/{function-path} [POST]
// @Summary Invoke Function By Path
// @Description Invoke Function By Path
// @Tags Function
// @Accept json
// @Produce json
// @Param InvokeFunctionByPathRequest body models.CommonMessage true "InvokeFunctionByPathRequest"
// @Success 201 {object} status_http.Response{data=models.InvokeFunctionRequest} "Function data"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV1) InvokeFunctionByPath(c *gin.Context) {
	var invokeFunction models.CommonMessage

	if err := c.ShouldBindJSON(&invokeFunction); err != nil {
		h.handleResponse(c, status_http.BadRequest, err.Error())
		return
	}

	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, status_http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		err := errors.New("error getting environment id | not valid")
		h.handleResponse(c, status_http.BadRequest, err)
		return
	}

	resource, err := h.companyServices.ServiceResource().GetSingle(
		c.Request.Context(),
		&pb.GetSingleServiceResourceReq{
			ProjectId:     projectId.(string),
			EnvironmentId: environmentId.(string),
			ServiceType:   pb.ServiceType_FUNCTION_SERVICE,
		},
	)
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}

	apiKeys, err := h.authService.ApiKey().GetList(context.Background(), &auth_service.GetListReq{
		EnvironmentId: environmentId.(string),
		ProjectId:     resource.ProjectId,
	})
	if err != nil {
		h.handleResponse(c, status_http.GRPCError, err.Error())
		return
	}
	if len(apiKeys.Data) < 1 {
		h.handleResponse(c, status_http.InvalidArgument, "Api key not found")
		return
	}
	authInfo, _ := h.GetAuthInfo(c)

	invokeFunction.Data["user_id"] = authInfo.GetUserId()
	invokeFunction.Data["project_id"] = authInfo.GetProjectId()
	invokeFunction.Data["environment_id"] = authInfo.GetEnvId()
	invokeFunction.Data["app_id"] = apiKeys.GetData()[0].GetAppId()

	resp, err := util.DoRequest("https://ofs.u-code.io/function/"+c.Param("function-path"), "POST", models.NewInvokeFunctionRequest{
		Data: invokeFunction.Data,
	})
	if err != nil {
		h.handleResponse(c, status_http.InvalidArgument, err.Error())
		return
	} else if resp.Status == "error" {
		var errStr = resp.Status
		if resp.Data != nil && resp.Data["message"] != nil {
			errStr = resp.Data["message"].(string)
		}
		h.handleResponse(c, status_http.InvalidArgument, errStr)
		return
	}

	h.handleResponse(c, status_http.Created, resp)
}
