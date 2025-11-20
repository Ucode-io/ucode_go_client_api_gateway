package v2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ucode/ucode_go_client_api_gateway/api/models"
	http "ucode/ucode_go_client_api_gateway/api/status_http"
	"ucode/ucode_go_client_api_gateway/config"
	pb "ucode/ucode_go_client_api_gateway/genproto/auth_service"
	pbc "ucode/ucode_go_client_api_gateway/genproto/company_service"
	pbSms "ucode/ucode_go_client_api_gateway/genproto/sms_service"
	"ucode/ucode_go_client_api_gateway/pkg/helper"
	"ucode/ucode_go_client_api_gateway/pkg/util"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spf13/cast"
)

// V2Register godoc
// @ID V2register
// @Router /v2/register [POST]
// @Summary V2Register
// @Description V2Register
// @Description in data must be have type, type must be one of the following values
// @Description ["google", "apple", "email", "phone"]
// @Description client_type_id and role_id must be in body parameters
// @Description you must be give environment_id and project_id in body or
// @Description Environment-Id hearder and project-id in query parameters or
// @Description X-API-KEY in hearder
// @Tags Auth
// @Accept json
// @Produce json
// @Param X-API-KEY header string false "X-API-KEY"
// @Param Resource-Id header string false "Resource-Id"
// @Param Environment-Id header string false "Environment-Id"
// @Param project-id query string false "project-id"
// @Param registerBody body models.RegisterOtp true "register_body"
// @Success 201 {object} status_http.Response{data=map[string]interface{}} "User data"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) Register(c *gin.Context) {
	var body models.RegisterOtp

	err := c.ShouldBindJSON(&body)
	if err != nil {
		h.handleResponse(c, http.BadRequest, err.Error())
		return
	}

	var (
		registerType  = body.Data["type"].(string)
		clientTypeId  = body.Data["client_type_id"].(string)
		roleId        = body.Data["role_id"].(string)
		projectId     = helper.AnyToString(c.Get("project_id"))
		environmentId = helper.AnyToString(c.Get("environment_id"))
		clientId      = helper.AnyToString(c.Get("client_id"))
	)

	for _, id := range []string{clientTypeId, roleId, projectId, environmentId} {
		if !util.IsValidUUID(id) {
			h.handleResponse(c, http.BadRequest, fmt.Sprintf("%s is an invalid uuid or not exist", id))
			return
		}
	}

	serviceResource, err := h.companyServices.ServiceResource().GetSingle(c.Request.Context(),
		&pbc.GetSingleServiceResourceReq{
			EnvironmentId: environmentId,
			ProjectId:     projectId,
			ServiceType:   pbc.ServiceType_BUILDER_SERVICE,
		},
	)
	if err != nil {
		h.handleResponse(c, http.GRPCError, err.Error())
		return
	}

	project, err := h.companyServices.Project().GetById(c.Request.Context(),
		&pbc.GetProjectByIdRequest{ProjectId: serviceResource.GetProjectId()})
	if err != nil {
		h.handleResponse(c, http.GRPCError, err.Error())
		return
	}

	switch registerType {
	case config.WithEmail:
		if value, ok := body.Data[config.WithEmail]; ok {
			if !util.IsValidEmail(value.(string)) {
				h.handleResponse(c, http.BadRequest, "Неверный формат email")
				return
			}
		} else {
			h.handleResponse(c, http.BadRequest, "Поле email не заполнено")
			return
		}
	case config.WithPhone:
		if _, ok := body.Data[config.WithPhone]; !ok {
			h.handleResponse(c, http.BadRequest, "Поле phone не заполнено")
			return
		}
	default:
		h.handleResponse(c, http.BadRequest, "register with google and apple not implemented")
		return

	}

	if value, ok := body.Data["addational_table"]; ok {
		if value.(map[string]interface{})["table_slug"] == nil {
			h.handleResponse(c, http.BadRequest, "If addional table have, table slug is required")
			return
		}
	}

	body.Data["company_id"] = project.GetCompanyId()
	body.Data["node_type"] = serviceResource.GetNodeType()
	body.Data["project_id"] = serviceResource.GetProjectId()
	body.Data["resource_type"] = serviceResource.GetResourceType()
	body.Data["environment_id"] = serviceResource.GetEnvironmentId()
	body.Data["resource_environment_id"] = serviceResource.GetResourceEnvironmentId()

	structData, err := helper.ConvertMapToStruct(body.Data)
	if err != nil {
		h.handleResponse(c, http.BadRequest, err.Error())
		return
	}

	response, err := h.authService.Register().RegisterUser(c.Request.Context(),
		&pb.RegisterUserRequest{
			Type:                  registerType,
			Data:                  structData,
			RoleId:                roleId,
			NodeType:              serviceResource.NodeType,
			ClientId:              clientId,
			CompanyId:             project.CompanyId,
			ProjectId:             serviceResource.ProjectId,
			ResourceId:            serviceResource.ResourceId,
			ClientTypeId:          clientTypeId,
			EnvironmentId:         serviceResource.EnvironmentId,
			ResourceEnvironmentId: serviceResource.ResourceEnvironmentId,
		})
	if err != nil {
		h.handleResponse(c, http.GRPCError, err.Error())
		return
	}

	h.handleResponse(c, http.Created, response)
}

// V2SendCode godoc
// @ID V2SendCode
// @Router /v2/send-code [POST]
// @Summary SendCode
// @Description SendCode type must be one of the following values ["EMAIL", "PHONE"]
// @Tags Auth
// @Accept json
// @Produce json
// @Param X-API-KEY header string false "X-API-KEY"
// @Param Resource-Id header string false "Resource-Id"
// @Param Environment-Id header string false "Environment-Id"
// @Param login body models.V2SendCodeRequest true "SendCode"
// @Success 201 {object} status_http.Response{data=models.V2SendCodeResponse} "User data"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) SendCode(c *gin.Context) {
	var (
		request models.V2SendCodeRequest
	)

	err := c.ShouldBindJSON(&request)
	if err != nil {
		h.handleResponse(c, http.BadRequest, err.Error())
		return
	}
	id, err := uuid.NewRandom()
	if err != nil {
		h.handleResponse(c, http.InternalServerError, err.Error())
		return
	}
	if !config.ValidRecipients[request.Type] {
		h.handleResponse(c, http.BadRequest, "Invalid recipient type")
		return
	}
	resourceId, ok := c.Get("resource_id")
	if !ok {
		h.handleResponse(c, http.BadRequest, errors.New("cant get resource_id").Error())
		return
	}
	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		h.handleResponse(c, http.BadRequest, errors.New("cant get environment_id").Error())
		return
	}
	expire := time.Now().Add(time.Minute * 5) // todo dont write expire time here

	resourceEnvironment, err := h.companyServices.Resource().GetResourceEnvironment(
		c.Request.Context(),
		&pbc.GetResourceEnvironmentReq{
			EnvironmentId: environmentId.(string),
			ResourceId:    resourceId.(string),
		},
	)
	if err != nil {
		h.handleResponse(c, http.GRPCError, err.Error())
		return
	}

	code, err := util.GenerateCode(4)
	if err != nil {
		h.handleResponse(c, http.InternalServerError, err.Error())
		return
	}
	body := &pbSms.Sms{
		Id:        id.String(),
		Text:      request.Text,
		Otp:       code,
		Recipient: request.Recipient,
		ExpiresAt: expire.String()[:19],
		Type:      request.Type,
	}

	switch request.Type {
	case "PHONE":
		valid := util.IsValidPhone(request.Recipient)
		if !valid {
			h.handleResponse(c, http.BadRequest, "Неверный номер телефона, он должен содержать двенадцать цифр и +")
			return
		}
		smsOtpSettings, err := h.companyServices.Resource().GetProjectResourceList(
			context.Background(),
			&pbc.GetProjectResourceListRequest{
				ProjectId:     resourceEnvironment.ProjectId,
				EnvironmentId: environmentId.(string),
				Type:          pbc.ResourceType_SMS,
			})
		if err != nil {
			h.handleResponse(c, http.GRPCError, err.Error())
			return
		}
		if len(smsOtpSettings.GetResources()) > 0 {
			if smsOtpSettings.GetResources()[0].GetSettings().GetSms().GetNumberOfOtp() != 0 {
				code, err := util.GenerateCode(int(smsOtpSettings.GetResources()[0].GetSettings().GetSms().GetNumberOfOtp()))
				if err != nil {
					h.handleResponse(c, http.InvalidArgument, "invalid number of otp")
					return
				}
				body.Otp = code
			}
			body.DevEmail = smsOtpSettings.GetResources()[0].GetSettings().GetSms().GetLogin()
			body.DevEmailPassword = smsOtpSettings.GetResources()[0].GetSettings().GetSms().GetPassword()
			body.Originator = smsOtpSettings.GetResources()[0].GetSettings().GetSms().GetOriginator()
		}
	case "EMAIL":
		valid := util.IsValidEmail(request.Recipient)
		if !valid {
			h.handleResponse(c, http.BadRequest, "Email is not valid")
			return
		}

		emailSettings, err := h.companyServices.Resource().GetProjectResourceList(
			context.Background(),
			&pbc.GetProjectResourceListRequest{
				ProjectId:     resourceEnvironment.ProjectId,
				EnvironmentId: environmentId.(string),
				Type:          pbc.ResourceType_SMTP,
			})
		if err != nil {
			h.handleResponse(c, http.GRPCError, err.Error())
			return
		}

		if len(emailSettings.GetResources()) < 1 {
			h.handleResponse(c, http.InvalidArgument, errors.New("email settings not found"))
			return
		}

		if len(emailSettings.GetResources()) > 0 {
			code, err := util.GenerateCode(int(emailSettings.GetResources()[0].GetSettings().GetSmtp().GetNumberOfOtp()))
			if err != nil {
				h.handleResponse(c, http.InvalidArgument, "invalid number of otp")
				return
			}
			body.Otp = code

			body.DevEmail = emailSettings.GetResources()[0].GetSettings().GetSmtp().GetEmail()
			body.DevEmailPassword = emailSettings.GetResources()[0].GetSettings().GetSmtp().GetPassword()
		}
	}

	services, err := h.GetProjectSrvc(
		c,
		resourceEnvironment.ProjectId,
		resourceEnvironment.NodeType,
	)
	if err != nil {
		h.handleResponse(c, http.GRPCError, err.Error())
		return
	}
	resp, err := services.SmsService().SmsService().Send(c.Request.Context(), body)
	if err != nil {
		h.handleResponse(c, http.GRPCError, err.Error())
		return
	}
	res := models.V2SendCodeResponse{
		SmsId: resp.SmsId,
	}

	h.handleResponse(c, http.Created, res)
}

// @Security ApiKeyAuth
// V2Login godoc
// @ID V2login
// @Param Resource-Id header string true "Resource-Id"
// @Param Environment-Id header string true "Environment-Id"
// @Router /v2/login [POST]
// @Summary V2Login
// @Description V2Login
// @Tags Auth
// @Accept json
// @Produce json
// @Param login body auth_service.V2LoginRequest true "LoginRequestBody"
// @Success 201 {object} status_http.Response{data=string} "User data"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) Login(c *gin.Context) {
	var (
		login pb.V2LoginRequest
		resp  *pb.V2LoginResponse
	)
	err := c.ShouldBindJSON(&login)
	if err != nil {
		h.handleResponse(c, http.BadRequest, err.Error())
		return
	}
	if login.ClientType == "" {
		h.handleResponse(c, http.BadRequest, "Необходимо выбрать тип пользователя")
		return
	}
	if login.ProjectId == "" {
		h.handleResponse(c, http.BadRequest, "Необходимо выбрать проекта")
		return
	}

	if login.Type == "" {
		login.Type = config.Default
	}

	switch login.Type {
	case config.Default:
		{
			if login.Username == "" {
				err := errors.New("username is required")
				h.handleResponse(c, http.BadRequest, err.Error())
				return
			}

			if login.Password == "" {
				err := errors.New("password is required")
				h.handleResponse(c, http.BadRequest, err.Error())
				return
			}
		}
	case config.WithPhone:
		{
			if login.SmsId == "" {
				err := errors.New("SmsId is required when type is not default")
				h.handleResponse(c, http.BadRequest, err.Error())
				return
			}

			if login.Otp == "" {
				err := errors.New("otp is required when type is not default")
				h.handleResponse(c, http.BadRequest, err.Error())
				return
			}

			if login.Phone == "" {
				err := errors.New("phone is required when type is phone")
				h.handleResponse(c, http.BadRequest, err.Error())
				return
			}
		}
	case config.WithEmail:
		{
			if login.SmsId == "" {
				err := errors.New("SmsId is required when type is not default")
				h.handleResponse(c, http.BadRequest, err.Error())
				return
			}

			if login.Otp == "" {
				err := errors.New("otp is required when type is not default")
				h.handleResponse(c, http.BadRequest, err.Error())
				return
			}

			if login.Email == "" {
				err := errors.New("email is required when type is email")
				h.handleResponse(c, http.BadRequest, err.Error())
				return
			}
		}
	}

	environmentId, ok := c.Get("environment_id")
	if !ok {
		h.handleResponse(c, http.BadRequest, errors.New("cant get environment_id"))
		return
	}

	clientId, ok := c.Get("client_id")
	if !ok {
		h.handleResponse(c, http.BadRequest, errors.New("cant get client_id"))
		return
	}

	resourceEnvironment, err := h.companyServices.ServiceResource().GetSingle(
		c.Request.Context(),
		&pbc.GetSingleServiceResourceReq{
			EnvironmentId: environmentId.(string),
			ProjectId:     login.GetProjectId(),
			ServiceType:   pbc.ServiceType_BUILDER_SERVICE,
		},
	)
	if err != nil {
		h.handleResponse(c, http.GRPCError, err.Error())
		return
	}

	login.ResourceEnvironmentId = resourceEnvironment.GetResourceEnvironmentId()
	login.ResourceType = int32(resourceEnvironment.GetResourceType())
	login.EnvironmentId = resourceEnvironment.GetEnvironmentId()
	login.NodeType = resourceEnvironment.GetNodeType()
	login.ClientId = cast.ToString(clientId)
	login.ClientIp = c.ClientIP()
	login.UserAgent = c.Request.UserAgent()

	service, conn, err := h.authService.Session(c)
	if err != nil {
		h.handleResponse(c, http.BadEnvironment, err.Error())
		return
	}
	defer conn.Close()

	resp, err = service.V2Login(c.Request.Context(), &login)
	httpErrorStr := ""
	if err != nil {
		httpErrorStr = strings.Split(err.Error(), "=")[len(strings.Split(err.Error(), "="))-1][1:]
		httpErrorStr = strings.ToLower(httpErrorStr)
	}
	if httpErrorStr == "user not found" {
		err := errors.New("пользователь не найдено")
		h.handleResponse(c, http.NotFound, err.Error())
		return
	} else if httpErrorStr == "user has been expired" {
		err := errors.New("срок действия пользователя истек")
		h.handleResponse(c, http.InvalidArgument, err.Error())
		return
	} else if httpErrorStr == "invalid username" {
		err := errors.New("неверное имя пользователя")
		h.handleResponse(c, http.InvalidArgument, err.Error())
		return
	} else if httpErrorStr == "invalid password" {
		err := errors.New("неверное пароль")
		h.handleResponse(c, http.InvalidArgument, err.Error())
		return
	} else if err != nil {
		h.handleResponse(c, http.GRPCError, err.Error())
		return
	}

	resp.EnvironmentId = resourceEnvironment.GetEnvironmentId()
	resp.ResourceId = resourceEnvironment.GetResourceId()

	h.handleResponse(c, http.Created, resp)
}

// V2LoginWithOption godoc
// @ID V2login_withoption
// @Router /v2/login/with-option [POST]
// @Summary V2LoginWithOption
// @Description V2LoginWithOption
// @Description inside the data you must be passed client_type_id field
// @Description you must be give environment_id and project_id in body or
// @Description Environment-Id hearder and project-id in query parameters or
// @Description X-API-KEY in hearder
// @Description login strategy must be one of the following values
// @Description ["EMAIL", "PHONE", "EMAIL_OTP", "PHONE_OTP", "LOGIN", "LOGIN_PWD", "GOOGLE_AUTH", "APPLE_AUTH", "PHONE_PWD", "EMAIL_PWD"]
// @Tags Auth
// @Accept json
// @Produce json
// @Param Environment-Id header string false "Environment-Id"
// @Param Client-Id header string false "Client-Id"
// @Param X-API-KEY header string false "X-API-KEY"
// @Param project-id query string false "project-id"
// @Param login body auth_service.V2LoginWithOptionRequest true "V2LoginRequest"
// @Success 201 {object} status_http.Response{data=map[string]interface{}} "User data"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) LoginWithOption(c *gin.Context) {
	var login pb.V2LoginWithOptionRequest
	err := c.ShouldBindJSON(&login)
	if err != nil {
		h.handleResponse(c, http.BadRequest, err.Error())
		return
	}
	clientType := login.Data["client_type_id"]
	if clientType == "" {
		h.handleResponse(c, http.InvalidArgument, "inside data client_type_id is required")
		return
	}
	if ok := util.IsValidUUID(clientType); !ok {
		h.handleResponse(c, http.InvalidArgument, "client_type_id is an invalid uuid")
		return
	}
	projectId, ok := c.Get("project_id")
	if !ok || !util.IsValidUUID(projectId.(string)) {
		h.handleResponse(c, http.InvalidArgument, "project id is an invalid uuid")
		return
	}

	environmentId, ok := c.Get("environment_id")
	if !ok || !util.IsValidUUID(environmentId.(string)) {
		err = errors.New("error getting environment id | not valid")
		h.handleResponse(c, http.BadRequest, err)
		return
	}
	login.Data["environment_id"] = environmentId.(string)
	login.Data["project_id"] = projectId.(string)

	service, conn, err := h.authService.Session(c)
	if err != nil {
		h.handleResponse(c, http.BadEnvironment, err.Error())
		return
	}
	defer conn.Close()

	resp, err := service.V2LoginWithOption(c.Request.Context(),
		&pb.V2LoginWithOptionRequest{
			Data:          login.GetData(),
			LoginStrategy: login.GetLoginStrategy(),
			Tables:        login.GetTables(),
			ClientIp:      c.ClientIP(),
			UserAgent:     c.Request.UserAgent(),
		})

	httpErrorStr := ""
	if err != nil {
		httpErrorStr = strings.Split(err.Error(), "=")[len(strings.Split(err.Error(), "="))-1][1:]
		httpErrorStr = strings.ToLower(httpErrorStr)

		if httpErrorStr == "user not found" {
			err := errors.New("пользователь не найдено")
			h.handleResponse(c, http.NotFound, err.Error())
			return
		} else if httpErrorStr == "user verified but not found" {
			err := errors.New("пользователь проверен, но не найден")
			h.handleResponse(c, http.OK, err.Error())
			return
		} else if httpErrorStr == "user has been expired" {
			err := errors.New("срок действия пользователя истек")
			h.handleResponse(c, http.InvalidArgument, err.Error())
			return
		} else if httpErrorStr == "invalid username" {
			err := errors.New("неверное имя пользователя")
			h.handleResponse(c, http.InvalidArgument, err.Error())
			return
		} else if httpErrorStr == "invalid password" {
			err := errors.New("неверное пароль")
			h.handleResponse(c, http.InvalidArgument, err.Error())
			return
		} else {
			h.handleResponse(c, http.GRPCError, err.Error())
			return
		}
	}

	res := &pb.V2LoginSuperAdminRes{
		UserFound: resp.GetUserFound(),
		Token:     resp.GetToken(),
		Companies: resp.GetCompanies(),
		UserId:    resp.GetUserId(),
		Sessions:  resp.GetSessions(),
		UserData:  resp.GetUserData(),
	}

	h.handleResponse(c, http.Created, res)
}

// V2Logout godoc
// @ID v2_logout
// @Router /v2/logout [POST]
// @Summary V2Logout User
// @Description V2Logout User
// @Tags Auth
// @Accept json
// @Produce json
// @Param data body auth_service.LogoutRequest true "LogoutRequest"
// @Success 204
// @Response 400 {object} status_http.Response{data=string} "Invalid Argument"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) Logout(c *gin.Context) {
	var logout pb.LogoutRequest

	err := c.ShouldBindJSON(&logout)
	if err != nil {
		h.handleResponse(c, http.BadRequest, err.Error())
		return
	}

	sessionService, conn, err := h.authService.Session(c)
	if err != nil {
		h.handleResponse(c, http.BadEnvironment, err.Error())
		return
	}
	defer conn.Close()

	resp, err := sessionService.Logout(c.Request.Context(), &logout)
	if err != nil {
		h.handleResponse(c, http.GRPCError, err.Error())
		return
	}

	h.handleResponse(c, http.NoContent, resp)
}

// V2ForgotPassword godoc
// @ID forgot_password
// @Router /v2/forgot-password [POST]
// @Summary ForgotPassword
// @Description Forgot Password
// @Tags Auth
// @Accept json
// @Produce json
// @Param login body auth_service.ForgotPasswordRequest true "ForgotPasswordRequest"
// @Success 201 {object} status_http.Response{data=models.ForgotPasswordResponse} "Response"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) ForgotPassword(c *gin.Context) {
	var (
		request pb.ForgotPasswordRequest
	)

	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second*60)
	defer cancel()

	err := c.ShouldBindJSON(&request)
	if err != nil {
		h.handleResponse(c, http.BadRequest, err.Error())
		return
	}

	user, err := h.authService.User().GetUserByUsername(ctx, &pb.GetUserByUsernameRequest{
		Username: request.Login,
	})
	if err != nil {
		h.handleResponse(c, http.GRPCError, err.Error())
		return
	}
	if user.GetEmail() == "" {
		h.handleResponse(c, http.OK, models.ForgotPasswordResponse{
			EmailFound: false,
			UserId:     user.GetId(),
			Email:      user.GetEmail(),
		})
		return
	}

	code, err := util.GenerateCode(6)
	if err != nil {
		h.handleResponse(c, http.InternalServerError, err.Error())
		return
	}
	expire := time.Now().Add(time.Hour * 5).Add(time.Minute * 5)

	id, err := uuid.NewRandom()
	if err != nil {
		h.handleResponse(c, http.InternalServerError, err.Error())
		return
	}

	resp, err := h.authService.Email().Create(
		c.Request.Context(),
		&pb.Email{
			Id:        id.String(),
			Email:     user.GetEmail(),
			Otp:       code,
			ExpiresAt: expire.String()[:19],
		},
	)
	if err != nil {
		h.handleResponse(c, http.GRPCError, err.Error())
		return
	}

	err = helper.SendCodeToEmail("Код для подтверждения", user.GetEmail(), code, h.baseConf.Email, h.baseConf.Password)
	if err != nil {
		h.handleResponse(c, http.InvalidArgument, err.Error())
		return
	}

	h.handleResponse(c, http.OK, models.ForgotPasswordResponse{
		EmailFound: true,
		SmsId:      resp.GetId(),
		UserId:     user.GetId(),
		Email:      user.GetEmail(),
	})
}

// RefreshToken godoc
// @ID refresh
// @Router /v2/refresh [PUT]
// @Summary V2Refresh Token
// @Description V2Refresh Token
// @Tags Auth
// @Accept json
// @Produce json
// @Param for_env query string false "for_env"
// @Param user body auth_service.RefreshTokenRequest true "RefreshTokenRequestBody"
// @Success 200 {object} status_http.Response{data=map[string]interface{}} "User data"
// @Response 400 {object} status_http.Response{data=string} "Bad Request"
// @Failure 500 {object} status_http.Response{data=string} "Server Error"
func (h *HandlerV2) RefreshToken(c *gin.Context) {
	var (
		user pb.RefreshTokenRequest
		resp *pb.V2LoginResponse
	)

	err := c.ShouldBindJSON(&user)
	if err != nil {
		h.handleResponse(c, http.BadRequest, err.Error())
		return
	}

	for_env := c.DefaultQuery("for_env", "")

	service, conn, err := h.authService.Session(c)
	if err != nil {
		h.handleResponse(c, http.BadEnvironment, err.Error())
		return
	}
	defer conn.Close()

	if for_env == "true" {
		resp, err = service.V2RefreshTokenForEnv(c.Request.Context(), &user)
		if err != nil {
			h.handleResponse(c, http.GRPCError, err.Error())
			return
		}
	} else {
		resp, err = service.V2RefreshToken(c.Request.Context(), &user)
		if err != nil {
			h.handleResponse(c, http.GRPCError, err.Error())
			return
		}
	}

	h.handleResponse(c, http.OK, resp)
}
