package main

import (
	// "bytes"
	"context"
	"fmt"

	// "io/ioutil"
	"os"
	// "strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var keyPairOutput *ec2.CreateKeyPairOutput
var mainKey = ""
var regionName = "us-east-1"
var bucketName = "go-aws"

func main() {
	var (
		instanceId string
		err        error
		s3Client   *s3.Client
		out        []byte
	)

	ctx := context.Background()
	if s3Client, err = inits3Client(ctx); err != nil {
		fmt.Printf("inits3Client error: %v", err)
		os.Exit(1)
	}
	fmt.Printf("s3Client: %v", s3Client)

	if err = createS3Bucket(ctx, s3Client); err != nil {
		fmt.Printf("CreateS3Bucket error %v ", err)
		os.Exit(1)
	}

	if err = uploadFile(ctx, s3Client); err != nil {
		fmt.Printf("uploadFile error %v ", err)
		os.Exit(1)
	}

	if out, err = downloadFile(ctx, s3Client); err != nil {
		fmt.Printf("uploadFile error %v ", err)
		os.Exit(1)
	}
	fmt.Printf("download complete: %v", out)

	if instanceId, err = createEC2(ctx); err != nil {
		fmt.Printf("createEC2 error: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Instance ID: %v", instanceId)
}

// EC2 Implementation

func createEC2(ctx context.Context) (string, error) {

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(regionName))
	if err != nil {
		return "", fmt.Errorf("unable to load SDK config, %v", err)
	}

	ec2Client := ec2.NewFromConfig(cfg)
	keyPairs, err := ec2Client.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{
		KeyNames: []string{"go-aws"},
	})

	if err != nil {
		return "", fmt.Errorf("describeKeyPairs error, %v", err)
	}

	if len(keyPairs.KeyPairs) == 1 {
		mainKey = *keyPairs.KeyPairs[0].KeyName
	}

	if keyPairs == nil || len(keyPairs.KeyPairs) == 0 {
		// var err error
		keyPairOutput, err = ec2Client.CreateKeyPair(ctx, &ec2.CreateKeyPairInput{
			KeyName: aws.String("go-aws"),
		})

		if err != nil {
			return "", fmt.Errorf("create keypair error, %v", err)
		}
		err = os.WriteFile("go-AWS.pem", []byte(*keyPairOutput.KeyMaterial), 0600)
		if err != nil {
			return "", fmt.Errorf("WriteFile error, %v", err)
		}
		mainKey = *keyPairOutput.KeyName
	}

	imageOutput, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("name"),
				Values: []string{"ubuntu/images/hvm-ssd/ubuntu-focal-20.04-amd64-server-*"},
			},
			{
				Name:   aws.String("virtualization-type"),
				Values: []string{"hvm"},
			},
		},
		Owners: []string{"099720109477"},
	})

	if err != nil {
		return "", fmt.Errorf("describe image error, %v", err)
	}

	if len(imageOutput.Images) == 0 {
		return "", fmt.Errorf("no image matches your description")
	}

	instance, err := ec2Client.RunInstances(ctx, &ec2.RunInstancesInput{
		ImageId:      imageOutput.Images[0].ImageId,
		KeyName:      aws.String(mainKey),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		InstanceType: types.InstanceTypeT3Micro,
	})

	if err != nil {
		return "", fmt.Errorf("a runInstances error")
	}

	if len(instance.Instances) == 0 {
		return "", fmt.Errorf("no imstance has been created")
	}

	return *instance.Instances[0].InstanceId, nil
}

// S3 Implementation
func inits3Client(ctx context.Context) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(regionName))
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config, %v", err)
	}

	return s3.NewFromConfig(cfg), nil
}

func createS3Bucket(ctx context.Context, s3Client *s3.Client) error {
	buckets, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return fmt.Errorf("listBuckets error, %v", err)
	}

	found := false
	for _, bucket := range buckets.Buckets {
		if *bucket.Name == bucketName {
			found = true
		}
	}
	if !found {
		_, err := s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucketName),
			CreateBucketConfiguration: &s3types.CreateBucketConfiguration{
				LocationConstraint: s3types.BucketLocationConstraint(regionName),
			},
		})
		if err != nil {
			return fmt.Errorf("createBucket error, %v", err)
		}

	}

	return nil
}

func uploadFile(ctx context.Context, s3Client *s3.Client) error {
	uploader := manager.NewUploader(s3Client)
	// textFile, err := ioutil.ReadFile("text.txt") // if we want to read a file from local system
	// if err != nil {
	// 	return fmt.Errorf("ReadFile error, %v", err)
	// }

	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String("text.txt"),
		// Body:   strings.NewReader("What the fuck!!!"),
		// Body: bytes.NewReader(textFile), /// if we want to read a file from local system
	})
	if err != nil {
		return fmt.Errorf("upload file error, %v", err)
	}
	return nil
}

func downloadFile(ctx context.Context, s3Client *s3.Client) ([]byte, error) {
	downloader := manager.NewDownloader(s3Client)
	buffer := manager.NewWriteAtBuffer([]byte{})

	numbytes, err := downloader.Download(ctx, buffer, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String("text.txt"),
	})

	if err != nil {
		return nil, fmt.Errorf("upload file error, %v", err)
	}

	if numbytesReceived := len(buffer.Bytes()); numbytes != int64(numbytesReceived) {
		return nil, fmt.Errorf("numbytesReceived doesnt match numbytes %d", numbytesReceived)
	}
	return buffer.Bytes(), nil
}
