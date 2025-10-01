package tasks

const (
	TaskTypeEmptyBucket = "empty_bucket"
	TaskTypeCopyBucket  = "copy_bucket"
)

type EmptyBucketPayload struct {
	UserID     string
	BucketName string
}

type CopyBucketPayload struct {
	UserID     string
	BucketSrc  string
	BucketDest string
}
