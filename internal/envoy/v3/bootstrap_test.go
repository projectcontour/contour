// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v3

import (
	"path"
	"testing"

	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
)

func TestBootstrap(t *testing.T) {
	tests := map[string]struct {
		config                        envoy.BootstrapConfig
		wantedBootstrapConfig         string
		wantedTLSCertificateConfig    string
		wantedValidationContextConfig string
		wantedError                   bool
	}{
		"default configuration": {
			config: envoy.BootstrapConfig{
				Path:      "envoy.json",
				Namespace: "testing-ns",
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_8001",
        "type": "STATIC",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 8001
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            }
          ]
        },
        "typed_extension_protocol_options": {
          "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
            "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
            "explicit_http_config": {
              "http2_protocol_options": {}
            }
          }
        },
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "envoy-admin",
        "alt_stat_name": "testing-ns_envoy-admin_9001",
        "type": "STATIC",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "envoy-admin",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "pipe": {
                        "path": "/admin/admin.sock",
                        "mode": "420"
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
	  "resource_api_version": "V3"
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
 	  "resource_api_version": "V3"
    }
  },
  "default_regex_engine": {
    "name": "envoy.regex_engines.google_re2",
    "typed_config": {
      "@type": "type.googleapis.com/envoy.extensions.regex_engines.v3.GoogleRE2"
    }
  },
  "admin": {
    "access_log": [
      {
        "name": "envoy.access_loggers.file",
        "typed_config": {
          "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
          "path": "/dev/null"
        }
      }
    ],
    "address": {
   	 "pipe": {
        "path": "/admin/admin.sock",
        "mode": "420"
      }
    }
  },
  "layered_runtime": {
    "layers": [
      {
        "name": "dynamic",
        "rtds_layer": {
          "name": "dynamic",
          "rtds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          }
        }
      },
      {
        "name": "admin",
        "admin_layer": {}
      }
    ]
  }
}`,
		},
		"--admin-address=someaddr": {
			config: envoy.BootstrapConfig{
				Path:         "envoy.json",
				AdminAddress: "someaddr",
				Namespace:    "testing-ns",
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_8001",
        "type": "STATIC",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 8001
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            }
          ]
        },
        "typed_extension_protocol_options": {
          "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
            "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
            "explicit_http_config": {
              "http2_protocol_options": {}
            }
          }
        },
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "envoy-admin",
        "alt_stat_name": "testing-ns_envoy-admin_9001",
        "type": "STATIC",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "envoy-admin",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                     "pipe": {
                        "path": "someaddr",
                        "mode": "420"
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
		"transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
      "resource_api_version": "V3"
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
		"transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
      "resource_api_version": "V3"
    }
  },
  "default_regex_engine": {
    "name": "envoy.regex_engines.google_re2",
    "typed_config": {
      "@type": "type.googleapis.com/envoy.extensions.regex_engines.v3.GoogleRE2"
    }
  },
  "admin": {
    "access_log": [
      {
        "name": "envoy.access_loggers.file",
        "typed_config": {
          "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
          "path": "/dev/null"
        }
      }
    ],
    "address": {
      "pipe": {
        "path": "someaddr",
        "mode": "420"
      }
    }
  },
  "layered_runtime": {
    "layers": [
      {
        "name": "dynamic",
        "rtds_layer": {
          "name": "dynamic",
          "rtds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          }
        }
      },
      {
        "name": "admin",
        "admin_layer": {}
      }
    ]
  }
}`,
		},
		"AdminAccessLogPath": { // TODO(dfc) doesn't appear to be exposed via contour bootstrap
			config: envoy.BootstrapConfig{
				Path:               "envoy.json",
				AdminAccessLogPath: "/var/log/admin.log",
				Namespace:          "testing-ns",
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_8001",
        "type": "STATIC",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 8001
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            }
          ]
        },
        "typed_extension_protocol_options": {
          "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
            "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
            "explicit_http_config": {
              "http2_protocol_options": {}
            }
          }
        },
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "envoy-admin",
        "alt_stat_name": "testing-ns_envoy-admin_9001",
        "type": "STATIC",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "envoy-admin",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "pipe": {
                        "path": "/admin/admin.sock",
                        "mode": "420"
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
		"transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
      "resource_api_version": "V3"
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
      "resource_api_version": "V3"
    }
  },
  "default_regex_engine": {
    "name": "envoy.regex_engines.google_re2",
    "typed_config": {
      "@type": "type.googleapis.com/envoy.extensions.regex_engines.v3.GoogleRE2"
    }
  },
  "admin": {
    "access_log": [
      {
        "name": "envoy.access_loggers.file",
        "typed_config": {
          "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
          "path": "/var/log/admin.log"
        }
      }
    ],
    "address": {
      "pipe": {
        "path": "/admin/admin.sock",
        "mode": "420"
      }
    }
  },
  "layered_runtime": {
    "layers": [
      {
        "name": "dynamic",
        "rtds_layer": {
          "name": "dynamic",
          "rtds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          }
        }
      },
      {
        "name": "admin",
        "admin_layer": {}
      }
    ]
  }
}`,
		},
		"--xds-address=8.8.8.8 --xds-port=9200": {
			config: envoy.BootstrapConfig{
				Path:        "envoy.json",
				XDSAddress:  "8.8.8.8",
				XDSGRPCPort: 9200,
				Namespace:   "testing-ns",
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_9200",
        "type": "STATIC",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "8.8.8.8",
                        "port_value": 9200
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            }
          ]
        },
        "typed_extension_protocol_options": {
          "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
            "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
            "explicit_http_config": {
              "http2_protocol_options": {}
            }
          }
        },
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "envoy-admin",
        "alt_stat_name": "testing-ns_envoy-admin_9001",
        "type": "STATIC",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "envoy-admin",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "pipe": {
                        "path": "/admin/admin.sock",
                        "mode": "420"
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
 		"transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
	  "resource_api_version": "V3"
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
 		"transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
	  "resource_api_version": "V3"
    }
  },
  "default_regex_engine": {
    "name": "envoy.regex_engines.google_re2",
    "typed_config": {
      "@type": "type.googleapis.com/envoy.extensions.regex_engines.v3.GoogleRE2"
    }
  },
  "admin": {
    "access_log": [
      {
        "name": "envoy.access_loggers.file",
        "typed_config": {
          "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
          "path": "/dev/null"
        }
      }
    ],
    "address": {
      "pipe": {
        "path": "/admin/admin.sock",
        "mode": "420"
      }
    }
  },
  "layered_runtime": {
    "layers": [
      {
        "name": "dynamic",
        "rtds_layer": {
          "name": "dynamic",
          "rtds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          }
        }
      },
      {
        "name": "admin",
        "admin_layer": {}
      }
    ]
  }
}`,
		},
		"--xds-address=contour --xds-port=9200": {
			config: envoy.BootstrapConfig{
				Path:        "envoy.json",
				XDSAddress:  "contour",
				XDSGRPCPort: 9200,
				Namespace:   "testing-ns",
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_9200",
        "type": "STRICT_DNS",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "contour",
                        "port_value": 9200
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            }
          ]
        },
        "typed_extension_protocol_options": {
          "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
            "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
            "explicit_http_config": {
              "http2_protocol_options": {}
            }
          }
        },
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "envoy-admin",
        "alt_stat_name": "testing-ns_envoy-admin_9001",
        "type": "STATIC",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "envoy-admin",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "pipe": {
                        "path": "/admin/admin.sock",
                        "mode": "420"
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
	  "resource_api_version": "V3"
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
	  "resource_api_version": "V3"
    }
  },
  "default_regex_engine": {
    "name": "envoy.regex_engines.google_re2",
    "typed_config": {
      "@type": "type.googleapis.com/envoy.extensions.regex_engines.v3.GoogleRE2"
    }
  },
  "admin": {
    "access_log": [
      {
        "name": "envoy.access_loggers.file",
        "typed_config": {
          "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
          "path": "/dev/null"
        }
      }
    ],
    "address": {
      "pipe": {
        "path": "/admin/admin.sock",
        "mode": "420"
      }
    }
  },
  "layered_runtime": {
    "layers": [
      {
        "name": "dynamic",
        "rtds_layer": {
          "name": "dynamic",
          "rtds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          }
        }
      },
      {
        "name": "admin",
        "admin_layer": {}
      }
    ]
  }
}`,
		},
		"--xds-address=::1 --xds-port=9200": {
			config: envoy.BootstrapConfig{
				Path:        "envoy.json",
				XDSAddress:  "::1",
				XDSGRPCPort: 9200,
				Namespace:   "testing-ns",
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_9200",
        "type": "STATIC",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "::1",
                        "port_value": 9200
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            }
          ]
        },
        "typed_extension_protocol_options": {
          "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
            "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
            "explicit_http_config": {
              "http2_protocol_options": {}
            }
          }
        },
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "envoy-admin",
        "alt_stat_name": "testing-ns_envoy-admin_9001",
        "type": "STATIC",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "envoy-admin",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "pipe": {
                        "path": "/admin/admin.sock",
                        "mode": "420"
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
	  "resource_api_version": "V3"
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
	  "resource_api_version": "V3"
    }
  },
  "default_regex_engine": {
    "name": "envoy.regex_engines.google_re2",
    "typed_config": {
      "@type": "type.googleapis.com/envoy.extensions.regex_engines.v3.GoogleRE2"
    }
  },
  "admin": {
    "access_log": [
      {
        "name": "envoy.access_loggers.file",
        "typed_config": {
          "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
          "path": "/dev/null"
        }
      }
    ],
    "address": {
      "pipe": {
        "path": "/admin/admin.sock",
        "mode": "420"
      }
    }
  },
  "layered_runtime": {
    "layers": [
      {
        "name": "dynamic",
        "rtds_layer": {
          "name": "dynamic",
          "rtds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          }
        }
      },
      {
        "name": "admin",
        "admin_layer": {}
      }
    ]
  }
}`,
		},
		"--xds-address=8.8.8.8 --xds-port=9200 --dns-lookup-family=v6": {
			config: envoy.BootstrapConfig{
				Path:            "envoy.json",
				XDSAddress:      "8.8.8.8",
				XDSGRPCPort:     9200,
				Namespace:       "testing-ns",
				DNSLookupFamily: "v6",
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_9200",
        "type": "STATIC",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "8.8.8.8",
                        "port_value": 9200
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            }
          ]
        },
        "typed_extension_protocol_options": {
          "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
            "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
            "explicit_http_config": {
              "http2_protocol_options": {}
            }
          }
        },
        "dns_lookup_family": "V6_ONLY",
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        }
      },
      {
        "name": "envoy-admin",
        "alt_stat_name": "testing-ns_envoy-admin_9001",
        "type": "STATIC",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "envoy-admin",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "pipe": {
                        "path": "/admin/admin.sock",
                        "mode": "420"
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
	  "resource_api_version": "V3"
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
	  "resource_api_version": "V3"
    }
  },
  "default_regex_engine": {
    "name": "envoy.regex_engines.google_re2",
    "typed_config": {
      "@type": "type.googleapis.com/envoy.extensions.regex_engines.v3.GoogleRE2"
    }
  },
  "admin": {
    "access_log": [
      {
        "name": "envoy.access_loggers.file",
        "typed_config": {
          "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
          "path": "/dev/null"
        }
      }
    ],
    "address": {
      "pipe": {
        "path": "/admin/admin.sock",
        "mode": "420"
      }
    }
  },
  "layered_runtime": {
    "layers": [
      {
        "name": "dynamic",
        "rtds_layer": {
          "name": "dynamic",
          "rtds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          }
        }
      },
      {
        "name": "admin",
        "admin_layer": {}
      }
    ]
  }
}`,
		},
		"--envoy-cafile=CA.cert --envoy-client-cert=client.cert --envoy-client-key=client.key": {
			config: envoy.BootstrapConfig{
				Path:              "envoy.json",
				Namespace:         "testing-ns",
				GrpcCABundle:      "CA.cert",
				GrpcClientCert:    "client.cert",
				GrpcClientKey:     "client.key",
				SkipFilePathCheck: true,
			},
			wantedBootstrapConfig: `{
  "static_resources": {
    "clusters": [
      {
        "name": "contour",
        "alt_stat_name": "testing-ns_contour_8001",
        "type": "STATIC",
        "connect_timeout": "5s",
        "load_assignment": {
          "cluster_name": "contour",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "socket_address": {
                        "address": "127.0.0.1",
                        "port_value": 8001
                      }
                    }
                  }
                }
              ]
            }
          ]
        },
        "circuit_breakers": {
          "thresholds": [
            {
              "priority": "HIGH",
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            },
            {
              "max_connections": 100000,
              "max_pending_requests": 100000,
              "max_requests": 60000000,
              "max_retries": 50,
              "track_remaining": true
            }
          ]
        },
        "typed_extension_protocol_options": {
          "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
            "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
            "explicit_http_config": {
              "http2_protocol_options": {}
            }
          }
        },
        "upstream_connection_options": {
          "tcp_keepalive": {
            "keepalive_probes": 3,
            "keepalive_time": 30,
            "keepalive_interval": 5
          }
        },
        "transport_socket": {
          "name": "envoy.transport_sockets.tls",
          "typed_config": {
            "@type":"type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext",
            "common_tls_context": {
              "tls_params": {
                "tls_maximum_protocol_version": "TLSv1_3"
              },
              "tls_certificates": [
                {
                  "certificate_chain": {
                    "filename": "client.cert"
                  },
                  "private_key": {
                    "filename": "client.key"
                  }
                }
              ],
              "validation_context": {
                "trusted_ca": {
                  "filename": "CA.cert"
                },
                "match_typed_subject_alt_names": [
                  {
                    "san_type": "DNS",
                    "matcher": {
                      "exact": "contour"
                    }
                  }
                ]
              }
            }
          }
        }
      },
      {
        "name": "envoy-admin",
        "alt_stat_name": "testing-ns_envoy-admin_9001",
        "type": "STATIC",
        "connect_timeout": "0.250s",
        "load_assignment": {
          "cluster_name": "envoy-admin",
          "endpoints": [
            {
              "lb_endpoints": [
                {
                  "endpoint": {
                    "address": {
                      "pipe": {
                        "path": "/admin/admin.sock",
                        "mode": "420"
                      }
                    }
                  }
                }
              ]
            }
          ]
        }
      }
    ]
  },
  "dynamic_resources": {
    "lds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
	  "resource_api_version": "V3"
    },
    "cds_config": {
      "api_config_source": {
        "api_type": "GRPC",
        "transport_api_version": "V3",
        "grpc_services": [
          {
            "envoy_grpc": {
              "cluster_name": "contour",
              "authority": "contour"
            }
          }
        ]
      },
	  "resource_api_version": "V3"
    }
  },
  "default_regex_engine": {
    "name": "envoy.regex_engines.google_re2",
    "typed_config": {
      "@type": "type.googleapis.com/envoy.extensions.regex_engines.v3.GoogleRE2"
    }
  },
  "admin": {
    "access_log": [
      {
        "name": "envoy.access_loggers.file",
        "typed_config": {
          "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
          "path": "/dev/null"
        }
      }
    ],
    "address": {
      "pipe": {
        "path": "/admin/admin.sock",
        "mode": "420"
      }
    }
  },
  "layered_runtime": {
    "layers": [
      {
        "name": "dynamic",
        "rtds_layer": {
          "name": "dynamic",
          "rtds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          }
        }
      },
      {
        "name": "admin",
        "admin_layer": {}
      }
    ]
  }
}`,
		},
		"--resources-dir tmp --envoy-cafile=CA.cert --envoy-client-cert=client.cert --envoy-client-key=client.key": {
			config: envoy.BootstrapConfig{
				Path:              "envoy.json",
				Namespace:         "testing-ns",
				ResourcesDir:      "resources",
				GrpcCABundle:      "CA.cert",
				GrpcClientCert:    "client.cert",
				GrpcClientKey:     "client.key",
				SkipFilePathCheck: true,
			},
			wantedBootstrapConfig: `{
        "static_resources": {
          "clusters": [
            {
              "name": "contour",
              "alt_stat_name": "testing-ns_contour_8001",
              "type": "STATIC",
              "connect_timeout": "5s",
              "load_assignment": {
                "cluster_name": "contour",
                "endpoints": [
                  {
                    "lb_endpoints": [
                      {
                        "endpoint": {
                          "address": {
                            "socket_address": {
                              "address": "127.0.0.1",
                              "port_value": 8001
                            }
                          }
                        }
                      }
                    ]
                  }
                ]
              },
              "circuit_breakers": {
                "thresholds": [
                  {
                    "priority": "HIGH",
                    "max_connections": 100000,
                    "max_pending_requests": 100000,
                    "max_requests": 60000000,
                    "max_retries": 50,
                    "track_remaining": true
                  },
                  {
                    "max_connections": 100000,
                    "max_pending_requests": 100000,
                    "max_requests": 60000000,
                    "max_retries": 50,
                    "track_remaining": true
                  }
                ]
              },
              "typed_extension_protocol_options": {
          "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
            "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
            "explicit_http_config": {
              "http2_protocol_options": {}
            }
          }
        },
              "transport_socket": {
                "name": "envoy.transport_sockets.tls",
                "typed_config": {
                  "@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext",
                  "common_tls_context": {
                    "tls_params": {
                      "tls_maximum_protocol_version": "TLSv1_3"
                    },
                    "tls_certificate_sds_secret_configs": [
                      {
                        "name": "contour_xds_tls_certificate",
                        "sds_config": {
                          "resource_api_version": "V3",
                          "path_config_source": {
                            "path": "resources/sds/xds-tls-certificate.json"
                          }
                        }
                      }
                    ],
                    "validation_context_sds_secret_config": {
                      "name": "contour_xds_tls_validation_context",
                      "sds_config": {
                        "resource_api_version": "V3",
                        "path_config_source": {
                          "path": "resources/sds/xds-validation-context.json"
                        }
                      }
                    }
                  }
                }
              },
              "upstream_connection_options": {
                "tcp_keepalive": {
                  "keepalive_probes": 3,
                  "keepalive_time": 30,
                  "keepalive_interval": 5
                }
              }
            },
            {
              "name": "envoy-admin",
              "alt_stat_name": "testing-ns_envoy-admin_9001",
              "type": "STATIC",
              "connect_timeout": "0.250s",
              "load_assignment": {
                "cluster_name": "envoy-admin",
                "endpoints": [
                  {
                    "lb_endpoints": [
                      {
                        "endpoint": {
                        "address": {
						  "pipe": {
							"path": "/admin/admin.sock",
							"mode": "420"
						  }
						}
                       }
					 }
                    ]
                  }
                ]
              }
            }
          ]
        },
        "dynamic_resources": {
          "lds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          },
          "cds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          }
        },
        "default_regex_engine": {
          "name": "envoy.regex_engines.google_re2",
          "typed_config": {
            "@type": "type.googleapis.com/envoy.extensions.regex_engines.v3.GoogleRE2"
          }
        },
        "admin": {
          "access_log": [
            {
              "name": "envoy.access_loggers.file",
              "typed_config": {
                "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
                "path": "/dev/null"
              }
            }
          ],
          "address": {
            "pipe": {
              "path": "/admin/admin.sock",
              "mode": "420"
            }
          }
        },
        "layered_runtime": {
          "layers": [
            {
              "name": "dynamic",
              "rtds_layer": {
                "name": "dynamic",
                "rtds_config": {
                  "api_config_source": {
                    "api_type": "GRPC",
                    "transport_api_version": "V3",
                    "grpc_services": [
                      {
                        "envoy_grpc": {
                          "cluster_name": "contour",
                          "authority": "contour"
                        }
                      }
                    ]
                  },
                  "resource_api_version": "V3"
                }
              }
            },
            {
              "name": "admin",
              "admin_layer": {}
            }
          ]
        }
    }`,
			wantedTLSCertificateConfig: `{
      "resources": [
        {
          "@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret",
          "name": "contour_xds_tls_certificate",
          "tls_certificate": {
            "certificate_chain": {
              "filename": "client.cert"
            },
            "private_key": {
              "filename": "client.key"
            }
          }
        }
      ]
    }`,
			wantedValidationContextConfig: `{
      "resources": [
        {
          "@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret",
          "name": "contour_xds_tls_validation_context",
          "validation_context": {
            "trusted_ca": {
              "filename": "CA.cert"
            },
            "match_typed_subject_alt_names": [
              {
                "san_type": "DNS",
                "matcher": {
                  "exact": "contour"
                }
              }
            ]
          }
        }
      ]
    }`,
		},
		"return error when not providing all certificate related parameters": {
			config: envoy.BootstrapConfig{
				Path:           "envoy.json",
				Namespace:      "testing-ns",
				ResourcesDir:   "resources",
				GrpcClientCert: "client.cert",
				GrpcClientKey:  "client.key",
			},
			wantedError: true,
		},
		"Enable overload manager by specifying --overload-max-heap=2147483648": {
			config: envoy.BootstrapConfig{
				Path:                 "envoy.json",
				Namespace:            "projectcontour",
				MaximumHeapSizeBytes: 2147483648, // 2 GiB
			},
			wantedBootstrapConfig: `{
        "static_resources": {
          "clusters": [
            {
              "name": "contour",
              "alt_stat_name": "projectcontour_contour_8001",
              "type": "STATIC",
              "connect_timeout": "5s",
              "load_assignment": {
                "cluster_name": "contour",
                "endpoints": [
                  {
                    "lb_endpoints": [
                      {
                        "endpoint": {
                          "address": {
                            "socket_address": {
                              "address": "127.0.0.1",
                              "port_value": 8001
                            }
                          }
                        }
                      }
                    ]
                  }
                ]
              },
              "circuit_breakers": {
                "thresholds": [
                  {
                    "priority": "HIGH",
                    "max_connections": 100000,
                    "max_pending_requests": 100000,
                    "max_requests": 60000000,
                    "max_retries": 50,
                    "track_remaining": true
                  },
                  {
                    "max_connections": 100000,
                    "max_pending_requests": 100000,
                    "max_requests": 60000000,
                    "max_retries": 50,
                    "track_remaining": true
                  }
                ]
              },
              "typed_extension_protocol_options": {
                "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
                  "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
                  "explicit_http_config": {
                    "http2_protocol_options": {}
                  }
                }
              },
              "upstream_connection_options": {
                "tcp_keepalive": {
                  "keepalive_probes": 3,
                  "keepalive_time": 30,
                  "keepalive_interval": 5
                }
              }
            },
            {
              "name": "envoy-admin",
              "alt_stat_name": "projectcontour_envoy-admin_9001",
              "type": "STATIC",
              "connect_timeout": "0.250s",
              "load_assignment": {
                "cluster_name": "envoy-admin",
                "endpoints": [
                  {
                    "lb_endpoints": [
                      {
                        "endpoint": {
                          "address": {
                            "pipe": {
                              "path": "/admin/admin.sock",
                              "mode": 420
                            }
                          }
                        }
                      }
                    ]
                  }
                ]
              }
            }
          ]
        },
        "default_regex_engine": {
          "name": "envoy.regex_engines.google_re2",
          "typed_config": {
            "@type": "type.googleapis.com/envoy.extensions.regex_engines.v3.GoogleRE2"
          }
        },
        "dynamic_resources": {
          "lds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          },
          "cds_config": {
            "api_config_source": {
              "api_type": "GRPC",
              "transport_api_version": "V3",
              "grpc_services": [
                {
                  "envoy_grpc": {
                    "cluster_name": "contour",
                    "authority": "contour"
                  }
                }
              ]
            },
            "resource_api_version": "V3"
          }
        },
        "layered_runtime": {
          "layers": [
            {
              "name": "dynamic",
              "rtds_layer": {
                "name": "dynamic",
                "rtds_config": {
                  "api_config_source": {
                    "api_type": "GRPC",
                    "transport_api_version": "V3",
                    "grpc_services": [
                      {
                        "envoy_grpc": {
                          "cluster_name": "contour",
                          "authority": "contour"
                        }
                      }
                    ]
                  },
                  "resource_api_version": "V3"
                }
              }
            },
            {
              "name": "admin",
              "admin_layer": {}
            }
          ]
        },
        "admin": {
          "access_log": [
            {
              "name": "envoy.access_loggers.file",
              "typed_config": {
                "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
                "path": "/dev/null"
              }
            }
          ],
          "address": {
            "pipe": {
              "path": "/admin/admin.sock",
              "mode": 420
            }
          }
        },
        "overload_manager": {
          "refresh_interval": "0.250s",
          "resource_monitors": [
            {
              "name": "envoy.resource_monitors.fixed_heap",
              "typed_config": {
                "@type": "type.googleapis.com/envoy.extensions.resource_monitors.fixed_heap.v3.FixedHeapConfig",
                "max_heap_size_bytes": "2147483648"
              }
            }
          ],
          "actions": [
            {
              "name": "envoy.overload_actions.shrink_heap",
              "triggers": [
                {
                  "name": "envoy.resource_monitors.fixed_heap",
                  "threshold": {
                    "value": 0.95
                  }
                }
              ]
            },
            {
              "name": "envoy.overload_actions.stop_accepting_requests",
              "triggers": [
                {
                  "name": "envoy.resource_monitors.fixed_heap",
                  "threshold": {
                    "value": 0.98
                  }
                }
              ]
            }
          ]
        }
      }`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc := tc
			envoyGen := NewEnvoyGen(EnvoyGenOpt{
				XDSClusterName: DefaultXDSClusterName,
			})
			steps, gotError := envoyGen.bootstrap(&tc.config)
			assert.Equal(t, tc.wantedError, gotError != nil)

			gotConfigs := map[string]proto.Message{}
			for _, step := range steps {
				path, config := step(&tc.config)
				gotConfigs[path] = config
			}

			sdsTLSCertificatePath := path.Join(tc.config.ResourcesDir, envoy.SDSResourcesSubdirectory, envoy.SDSTLSCertificateFile)
			sdsValidationContextPath := path.Join(tc.config.ResourcesDir, envoy.SDSResourcesSubdirectory, envoy.SDSValidationContextFile)

			if tc.wantedBootstrapConfig != "" {
				want := new(envoy_config_bootstrap_v3.Bootstrap)
				unmarshal(t, tc.wantedBootstrapConfig, want)
				protobuf.ExpectEqual(t, want, gotConfigs[tc.config.Path])
				delete(gotConfigs, tc.config.Path)
			}

			if tc.wantedTLSCertificateConfig != "" {
				want := new(envoy_service_discovery_v3.DiscoveryResponse)
				unmarshal(t, tc.wantedTLSCertificateConfig, want)
				protobuf.ExpectEqual(t, want, gotConfigs[sdsTLSCertificatePath])
				delete(gotConfigs, sdsTLSCertificatePath)
			}

			if tc.wantedValidationContextConfig != "" {
				want := new(envoy_service_discovery_v3.DiscoveryResponse)
				unmarshal(t, tc.wantedValidationContextConfig, want)
				protobuf.ExpectEqual(t, want, gotConfigs[sdsValidationContextPath])
				delete(gotConfigs, sdsValidationContextPath)
			}

			if len(gotConfigs) > 0 {
				t.Fatalf("got more configs than wanted: %s", gotConfigs)
			}
		})
	}
}

func unmarshal(t *testing.T, data string, pb proto.Message) {
	err := protojson.Unmarshal([]byte(data), pb)
	checkErr(t, err)
}

func checkErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
