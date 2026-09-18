package api

import "context"

// uploadRecordingObject writes to the one remote recording store selected at
// startup. Test deployments select NAS; all other deployments retain OCI/S3.
// A configured store is never bypassed for another provider after an error.
func (s *Server) uploadRecordingObject(ctx context.Context, key string, data []byte) (url, provider string, err error) {
	if s.nas != nil {
		url, err = s.nas.UploadPublic(ctx, key, data)
		return url, "NAS", err
	}
	if s.oci != nil {
		url, err = s.oci.UploadPublic(ctx, key, data)
		return url, "OCI", err
	}
	if s.s3 != nil {
		url, err = s.s3.UploadPublic(ctx, key, data)
		return url, "S3", err
	}
	return "", "", nil
}
