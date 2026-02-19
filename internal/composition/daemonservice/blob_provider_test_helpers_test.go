package daemonservice

func useSharedBlobProviders(registry *blobProviderRegistry, services ...*Service) {
	for _, svc := range services {
		if svc == nil {
			continue
		}
		svc.blobProviders = registry
	}
}
