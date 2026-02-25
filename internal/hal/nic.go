package hal

type NIC interface {
	Request(req map[string]any) error
	RequestResponse(req map[string]any) (map[string]any, error)
}
