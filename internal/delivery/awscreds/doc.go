// Package awscreds finds the AWS credentials the SNS and SES providers
// sign with: static keys from the configuration, IAM roles for service
// accounts (web identity, exchanged at STS) or EKS Pod Identity (the
// node's Pod Identity agent). Temporary credentials are cached and
// fetched again a few minutes before they expire. The EC2 instance role
// (IMDS) is deliberately not used: kwatch refuses the metadata address.
package awscreds
