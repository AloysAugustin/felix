// Copyright (c) 2020 Tigera, Inc. All rights reserved.

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

package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	testRegion = "us-west-2"
	testEniId  = "eni-i-000"
	testInstId = "i-000"
)

type mockClient struct {
	ec2iface.EC2API
	ec2MetadaAPI
	UsageCounter int
}

func newMockClient() *mockClient {
	return &mockClient{UsageCounter: 0}
}

func (c *mockClient) GetInstanceIdentityDocumentWithContext(ctx aws.Context) (ec2metadata.EC2InstanceIdentityDocument, error) {
	c.UsageCounter++

	return ec2metadata.EC2InstanceIdentityDocument{
		InstanceID: testInstId,
	}, nil
}

func (c *mockClient) RegionWithContext(ctx aws.Context) (string, error) {
	c.UsageCounter++

	return testRegion, nil
}

func (c *mockClient) ModifyNetworkInterfaceAttributeWithContext(ctx aws.Context, input *ec2.ModifyNetworkInterfaceAttributeInput, opts ...request.Option) (*ec2.ModifyNetworkInterfaceAttributeOutput, error) {
	c.UsageCounter++

	return nil, nil
}

func (c *mockClient) DescribeInstancesWithContext(ctx aws.Context, input *ec2.DescribeInstancesInput, opts ...request.Option) (*ec2.DescribeInstancesOutput, error) {
	c.UsageCounter++

	deviceIndexZero := int64(0)
	eniId := testEniId

	return &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			{
				Instances: []*ec2.Instance{
					{
						NetworkInterfaces: []*ec2.InstanceNetworkInterface{
							{
								Attachment: &ec2.InstanceNetworkInterfaceAttachment{
									DeviceIndex: &deviceIndexZero,
								},
								NetworkInterfaceId: &eniId,
							},
						},
					},
				},
			},
		},
	}, nil
}

var _ = Describe("AWS Tests", func() {
	It("should correctly convert between errors and awserrors", func() {
		fakeCode := "fakeCode"
		fakeMsg := "fakeMsg"

		awsErr := awserr.New(fakeCode, fakeMsg, nil)
		errMsg := convertError(awsErr)
		Expect(errMsg).To(Equal(fmt.Sprintf("%s: %s", fakeCode, fakeMsg)))

		fakeMsg = "fake non-aws error"
		err := fmt.Errorf(fakeMsg)
		errMsg = convertError(err)
		Expect(errMsg).To(Equal(fakeMsg))
	})

	It("should handle retriable server error", func() {
		internalErrCode := "InternalError"
		internalErrMsg := "internal error"

		awsErr := awserr.New(internalErrCode, internalErrMsg, nil)
		Expect(retriable(awsErr)).To(BeTrue())

		fakeCode := "fakeCode"
		fakeMsg := "fakeMsg"

		awsErr = awserr.New(fakeCode, fakeMsg, nil)
		Expect(retriable(awsErr)).To(BeFalse())

		fakeMsg = "non-aws error"
		err := fmt.Errorf(fakeMsg)
		Expect(retriable(err)).To(BeFalse())
	})

	It("should handle EC2Metadata interactions correctly", func() {
		mock := newMockClient()
		Expect(getEC2InstanceID(context.TODO(), mock)).To(Equal(testInstId))
		Expect(getEC2Region(context.TODO(), mock)).To(Equal(testRegion))
		Expect(mock.UsageCounter).To(BeNumerically("==", 2))
	})

	It("should handle EC2 interactions correctly", func() {
		mock := newMockClient()
		cli := &ec2Client{
			EC2Svc:        mock,
			ec2InstanceId: testInstId,
		}

		Expect(cli.getEC2NetworkInterfaceId(context.TODO())).To(Equal(testEniId))
		Expect(cli.setEC2SourceDestinationCheck(context.TODO(), testEniId, false)).NotTo(HaveOccurred())
		Expect(mock.UsageCounter).To(BeNumerically("==", 2))

		By("verifying Availability")
		os.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		_, err := newEC2Client(context.TODO())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("EC2 metadata service is unavailable"))
	})
})
