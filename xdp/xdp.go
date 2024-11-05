package xdp

func GetObjects() (*xdpObjects, error) {
	var objs xdpObjects
	if err := loadXdpObjects(&objs, nil); err != nil {
		return nil, err
	}
	return &objs, nil
}
