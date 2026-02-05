package config

import "testing"

func TestSealedOverlayPrecedence(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:       "primary",
			Deployment: "prod",
			Partitions: []string{"p1"},
		},
		Stacks: map[string]Stack{
			"core": {
				Services: map[string]Service{
					"api": {Image: "base"},
				},
			},
		},
		Overlays: Overlays{
			Deployments: map[string]Overlay{
				"prod": {
					Stacks: map[string]OverlayStack{
						"core": {
							Sealed: true,
							Services: map[string]OverlayService{
								"api": {
									Fields: map[string]any{
										"image": "deploy",
									},
								},
							},
						},
					},
				},
			},
			Partitions: PartitionOverlays{
				Rules: []PartitionOverlay{
					{
						Name:  "p1",
						Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
						Overlay: Overlay{
							Stacks: map[string]OverlayStack{
								"core": {
									Services: map[string]OverlayService{
										"api": {
											Sealed: true,
											Fields: map[string]any{
												"image": "partition",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	services, err := cfg.StackServices("core", "p1")
	if err != nil {
		t.Fatalf("stack services: %v", err)
	}
	service, ok := services["api"]
	if !ok {
		t.Fatalf("expected api service")
	}
	if service.Image != "deploy" {
		t.Fatalf("expected sealed deployment overlay to win; got %q", service.Image)
	}
}

func TestOverlayPrecedenceUnsealed(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:       "primary",
			Deployment: "prod",
			Partitions: []string{"p1"},
		},
		Stacks: map[string]Stack{
			"core": {
				Services: map[string]Service{
					"api": {
						Image: "base",
						Overlays: ServiceOverlays{
							Deployments: map[string]OverlayService{
								"prod": {
									Fields: map[string]any{"image": "service-deploy"},
								},
							},
							Partitions: ServicePartitionOverlays{
								Rules: []ServicePartitionOverlay{
									{
										Name:  "p1",
										Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
										Service: OverlayService{
											Fields: map[string]any{"image": "service-partition"},
										},
									},
								},
							},
						},
					},
				},
				Overlays: StackOverlays{
					Deployments: map[string]OverlayStack{
						"prod": {
							Services: map[string]OverlayService{
								"api": {Fields: map[string]any{"image": "stack-deploy"}},
							},
						},
					},
					Partitions: StackPartitionOverlays{
						Rules: []StackPartitionOverlay{
							{
								Name:  "p1",
								Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
								OverlayStack: OverlayStack{
									Services: map[string]OverlayService{
										"api": {Fields: map[string]any{"image": "stack-partition"}},
									},
								},
							},
						},
					},
				},
			},
		},
		Overlays: Overlays{
			Deployments: map[string]Overlay{
				"prod": {
					Stacks: map[string]OverlayStack{
						"core": {
							Services: map[string]OverlayService{
								"api": {Fields: map[string]any{"image": "project-deploy"}},
							},
						},
					},
				},
			},
			Partitions: PartitionOverlays{
				Rules: []PartitionOverlay{
					{
						Name:  "p1",
						Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
						Overlay: Overlay{
							Stacks: map[string]OverlayStack{
								"core": {
									Services: map[string]OverlayService{
										"api": {Fields: map[string]any{"image": "project-partition"}},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	services, err := cfg.StackServices("core", "p1")
	if err != nil {
		t.Fatalf("stack services: %v", err)
	}
	service, ok := services["api"]
	if !ok {
		t.Fatalf("expected api service")
	}
	if service.Image != "service-partition" {
		t.Fatalf("expected service partition overlay to win; got %q", service.Image)
	}
}

func TestOverlayPrecedenceSealedProjectWins(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:       "primary",
			Deployment: "prod",
			Partitions: []string{"p1"},
		},
		Stacks: map[string]Stack{
			"core": {
				Services: map[string]Service{
					"api": {
						Image: "base",
						Overlays: ServiceOverlays{
							Deployments: map[string]OverlayService{
								"prod": {
									Sealed: true,
									Fields: map[string]any{"image": "service-sealed"},
								},
							},
						},
					},
				},
				Overlays: StackOverlays{
					Deployments: map[string]OverlayStack{
						"prod": {
							Sealed: true,
							Services: map[string]OverlayService{
								"api": {Fields: map[string]any{"image": "stack-sealed"}},
							},
						},
					},
				},
			},
		},
		Overlays: Overlays{
			Deployments: map[string]Overlay{
				"prod": {
					Stacks: map[string]OverlayStack{
						"core": {
							Sealed: true,
							Services: map[string]OverlayService{
								"api": {Fields: map[string]any{"image": "project-sealed"}},
							},
						},
					},
				},
			},
		},
	}

	services, err := cfg.StackServices("core", "p1")
	if err != nil {
		t.Fatalf("stack services: %v", err)
	}
	service, ok := services["api"]
	if !ok {
		t.Fatalf("expected api service")
	}
	if service.Image != "project-sealed" {
		t.Fatalf("expected project sealed overlay to win; got %q", service.Image)
	}
}

func TestOverlayPrecedenceSealedStackWinsOverService(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:       "primary",
			Deployment: "prod",
			Partitions: []string{"p1"},
		},
		Stacks: map[string]Stack{
			"core": {
				Services: map[string]Service{
					"api": {
						Image: "base",
						Overlays: ServiceOverlays{
							Partitions: ServicePartitionOverlays{
								Rules: []ServicePartitionOverlay{
									{
										Name:  "p1",
										Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
										Service: OverlayService{
											Sealed: true,
											Fields: map[string]any{"image": "service-sealed"},
										},
									},
								},
							},
						},
					},
				},
				Overlays: StackOverlays{
					Partitions: StackPartitionOverlays{
						Rules: []StackPartitionOverlay{
							{
								Name:  "p1",
								Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "p1"}},
								OverlayStack: OverlayStack{
									Sealed: true,
									Services: map[string]OverlayService{
										"api": {Fields: map[string]any{"image": "stack-sealed"}},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	services, err := cfg.StackServices("core", "p1")
	if err != nil {
		t.Fatalf("stack services: %v", err)
	}
	service, ok := services["api"]
	if !ok {
		t.Fatalf("expected api service")
	}
	if service.Image != "stack-sealed" {
		t.Fatalf("expected stack sealed overlay to win; got %q", service.Image)
	}
}
