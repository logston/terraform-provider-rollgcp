variable "region" {
  default = "us-central1"
}

variable "region_zone" {
  default = "us-central1-f"
}

variable "org_id" {
  description = "The ID of the Google Cloud Organization."
}

variable "project_id" {
  description = "The ID of the Google Cloud project."
}

variable "project_name" {
  description = "The name of the Google Cloud project."
}

variable "cluster_name" {
  default = "rollable-node-pool-cluster"
}

variable "node_pool_name" {
  default = "rollable-node-pool-cluster"
}

variable "billing_account_id" {
  description = "The ID of the associated billing account (optional)."
}

variable "credentials_file_path" {
  description = "Location of the credentials to use."
  default     = "~/.gcloud/gcp.json"
}
