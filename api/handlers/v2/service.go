package v2

import (
	"ucode/ucode_go_client_api_gateway/services"
)

func (h *HandlerV2) GetService(namespace string) (services.ServiceManagerI, error) {
	return h.services.Get(namespace)
}

func (h *HandlerV2) RemoveService(namespace string) error {
	return h.services.Remove(namespace)
}
