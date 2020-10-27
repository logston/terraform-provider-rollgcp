# terraform-provider-rolleksgcp

This Terraform provider allows for a GKE node pool to be updated without any downtime.

### How it works

The provider works by defining a new resource called a
`rolleksgcp_container_node_pool`. This resource all of the same attributes and
configuration features of a `google_container_node_pool` from the
https://github.com/hashicorp/terraform-provider-google-beta/ provider.

When terraform tells the this provider to update a node pool, this provider ...

1. creates a temporary node pool
1. cordons the original node pool in order to move the pods to the temporary node pool
1. deletes the original node pool
1. creates a new node pool with the same name as the original node pool
1. cordons the temporary node pool in order to move the pods to the new node pool
1. deletes the temporary node pool

Why not just create a new node pool and move the pods once? Why the need for
the temporary node pool? Doing so would leave the state of the live node pool
with a different name than what is defined in the declarative TF file. This
would mean that the next time TF is run, it would think that a node pool is
gone and try to create a new node pool by the name defined in the TF file.

Can we create two node pools with the same name and side-step the issue above?
No. Names must be unique.

### Test It Out

1. Get a service account key from GCP for the project you are going to use. Place that key at `~/.gcloud/gcp.json`.
1. Clone repo
1. Run `make install`
1. `cd examples/node-group`
1. Update the variables file to point at a GCP project of your choosing
1. `terraform init && terraform apply  --auto-approve` (This will create the cluster and node pool to be rolled)
1. Change the `machine_type` from `"e2-small"` to `"e2-medium"`
1. `terraform init && terraform apply --auto-approve` (This will roll the node pool up to the new size)
1. Watch in GCP or watch in kubectl logs if you have a pod running

### What does it look like in use?

In a main.tf file, something kind of like this...
```hcl
resource "google_container_cluster" "primary" {
  name     = var.cluster_name
  location = var.region
  project  = var.project_id
}

resource "rolleksgcp_container_node_pool" "primary_node_pool" {
  project    = var.project_id
  name       = var.node_pool_name
  location   = var.region
  cluster    = google_container_cluster.primary.name
  node_count = 1

  node_config {
    preemptible  = true
    machine_type = "e2-small"

    oauth_scopes = [
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
    ]
  }
}
```

See the examples for a full example.

### Can Pulumi use this?

Yep. Another layer of wrapping will be needed though. Pulumi provides a
tf2pulumi tool to generate the pulumi plugin from a terraform provider, thus it
may be easy to do.

### What's left to do?

- Unit tests
- Wait for cordoning to complete. Currently, there is no logic to wait for a
  node pool to be drained before it is deleted. This will be implemented if
  this tool is chosen as the method for zero downtime migrations of GCP node
  pools.
