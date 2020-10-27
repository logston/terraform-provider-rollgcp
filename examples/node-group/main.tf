terraform {
  required_providers {
    rolleksgcp = {
      versions = ["0.1"]
      source = "hashicorp.com/edu/rolleksgcp"
    }
  }
}

provider "google" {
  region      = var.region
  credentials = file(var.credentials_file_path)
}

provider "rolleksgcp" {
  region      = var.region
  credentials = file(var.credentials_file_path)
}

resource "google_container_cluster" "primary" {
  name     = var.cluster_name
  location = var.region
  project  = var.project_id

  # We can't create a cluster with no node pool defined, but we want to only use
  # separately managed node pools. So we create the smallest possible default
  # node pool and immediately delete it.
  remove_default_node_pool = true
  initial_node_count       = 1
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
