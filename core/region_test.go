// Copyright (c) 2016-2019 Cristian Măgherușan-Stanciu
// Licensed under the Open Software License version 3.0

package autospotting

import (
	"math"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ec2"
	ec2instancesinfo "github.com/vkhodor/ec2-instances-info"
	"github.com/davecgh/go-spew/spew"
)

func Test_region_enabled(t *testing.T) {

	tests := []struct {
		name    string
		region  string
		allowed string
		want    bool
	}{
		{
			name:    "No regions given in the filter",
			region:  "us-east-1",
			allowed: "",
			want:    true,
		},
		{
			name:    "Running in a different region than one allowed one",
			region:  "us-east-1",
			allowed: "eu-west-1",
			want:    false,
		},

		{
			name:    "Running in a different region than a list of allowed ones",
			region:  "us-east-1",
			allowed: "eu-west-1 ca-central-1",
			want:    false,
		},
		{
			name:    "Running in a region from the allowed ones",
			region:  "us-east-1",
			allowed: "us-east-1 eu-west-1",
			want:    true,
		},
		{
			name:    "Comma-separated allowed regions",
			region:  "us-east-1",
			allowed: "us-east-1,eu-west-1",
			want:    true,
		},
		{
			name:    "Comma and whitespace-separated allowed regions",
			region:  "us-east-1",
			allowed: "us-east-1, eu-west-1",
			want:    true,
		},
		{
			name:    "Whitespace-and-comma-separated allowed regions",
			region:  "us-east-1",
			allowed: "us-east-1, eu-west-1",
			want:    true,
		},
		{
			name:    "Region globs matching",
			region:  "us-east-1",
			allowed: "us-*, eu-*",
			want:    true,
		},
		{
			name:    "Region globs not matching",
			region:  "us-east-1",
			allowed: "ap-*, eu-*",
			want:    false,
		},
		{
			name:    "Region globs without dash matching",
			region:  "us-east-1",
			allowed: "us*, eu*",
			want:    true,
		},
		{
			name:    "Region globs without dash not matching",
			region:  "us-east-1",
			allowed: "ap*, eu*",
			want:    false,
		},
		{
			name:    "Non-separated allowed regions",
			region:  "us-east-1",
			allowed: "us-east-1eu-west-1",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &region{
				name: tt.region,
				conf: &Config{
					Regions: tt.allowed,
				},
			}
			if got := r.enabled(); got != tt.want {
				t.Errorf("region.enabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAsgFiltersSetupOnRegion(t *testing.T) {
	tests := []struct {
		name    string
		want    []Tag
		tregion *region
	}{
		{
			name: "No tags specified",
			want: []Tag{{Key: "spot-enabled", Value: "true"}},
			tregion: &region{
				conf: &Config{},
			},
		},
		{
			name: "No tags specified",
			want: []Tag{{Key: "spot-enabled", Value: "true"}, {Key: "environment", Value: "dev"}},
			tregion: &region{
				conf: &Config{
					FilterByTags: "spot-enabled=true, environment=dev",
				},
			},
		},
		{
			name: "No tags specified",
			want: []Tag{{Key: "spot-enabled", Value: "true"}, {Key: "environment", Value: "dev"}, {Key: "team", Value: "interactive"}},
			tregion: &region{
				conf: &Config{
					FilterByTags: "spot-enabled=true, environment=dev,team=interactive",
				},
			},
		},
	}
	for _, tt := range tests {

		tt.tregion.setupAsgFilters()
		if !reflect.DeepEqual(tt.want, tt.tregion.tagsToFilterASGsBy) {
			t.Errorf("region.setupAsgFilters() = %v, want %v", tt.tregion.tagsToFilterASGsBy, tt.want)

		}

	}
}

func TestOnDemandPriceMultiplier(t *testing.T) {
	tests := []struct {
		multiplier float64
		want       float64
	}{
		{
			multiplier: 1.0,
			want:       0.044,
		},
		{
			multiplier: 2.0,
			want:       0.088,
		},
		{
			multiplier: 0.99,
			want:       0.04356,
		},
	}
	for _, tt := range tests {
		cfg := &Config{
			InstanceData: &ec2instancesinfo.InstanceData{
				0: {
					InstanceType: "m1.small",
					Pricing: map[string]ec2instancesinfo.RegionPrices{
						"us-east-1": {
							Linux: ec2instancesinfo.LinuxPricing{
								OnDemand: 0.044,
							},
						},
					},
				},
			},
			AutoScalingConfig: AutoScalingConfig{
				OnDemandPriceMultiplier: tt.multiplier,
			},
		}
		r := region{
			name: "us-east-1",
			conf: cfg,
			services: connections{
				ec2: mockEC2{
					dsphpo: []*ec2.DescribeSpotPriceHistoryOutput{
						{
							SpotPriceHistory: []*ec2.SpotPrice{},
						},
					},
				},
			}}
		r.determineInstanceTypeInformation(cfg)

		actualPrice := r.instanceTypeInformation["m1.small"].pricing.onDemand
		if math.Abs(actualPrice-tt.want) > 0.000001 {
			t.Errorf("multiplier = %.2f, pricing.onDemand = %.5f, want %.5f",
				tt.multiplier, actualPrice, tt.want)
		}
	}
}

func TestDefaultASGFiltering(t *testing.T) {
	tests := []struct {
		tregion  *region
		expected []Tag
	}{
		{
			expected: []Tag{{Key: "spot-enabled", Value: "true"}},
			tregion: &region{
				conf: &Config{
					FilterByTags: "bob",
				},
			},
		},
		{
			expected: []Tag{{Key: "bob", Value: "value"}},
			tregion: &region{
				conf: &Config{
					FilterByTags: "bob=value",
				},
			},
		},
		{
			expected: []Tag{{Key: "spot-enabled", Value: "true"}, {Key: "team", Value: "interactive"}},
			tregion: &region{
				conf: &Config{
					FilterByTags: "spot-enabled=true,team=interactive",
				},
			},
		},
	}
	for _, tt := range tests {
		tt.tregion.setupAsgFilters()
		for _, tag := range tt.expected {
			matchingTag := false
			for _, setTag := range tt.tregion.tagsToFilterASGsBy {
				if tag.Key == setTag.Key && tag.Value == setTag.Value {
					matchingTag = true
				}
			}

			if !matchingTag {
				t.Errorf("tags not correctly filtered = %v, want %v", tt.tregion.tagsToFilterASGsBy, tt.expected)

			}
		}
	}
}

func TestFilterAsgs(t *testing.T) {
	// Test invalid regular expression
	var nullSlice []string
	stackName := "dummyStackName"
	stackStatus := "UPDATE_COMPLETE"

	tests := []struct {
		name    string
		want    []string
		tregion *region
	}{
		{
			name: "Test with single filter",
			want: []string{"asg1", "asg2", "asg3", "asg4"},
			tregion: &region{
				tagsToFilterASGsBy: []Tag{{Key: "spot-enabled", Value: "true"}},
				conf:               &Config{},
				services: connections{
					autoScaling: mockASG{
						dasgo: &autoscaling.DescribeAutoScalingGroupsOutput{
							AutoScalingGroups: []*autoscaling.Group{
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg1")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg1")},
									},
									AutoScalingGroupName: aws.String("asg1"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg2")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg2")},
									},
									AutoScalingGroupName: aws.String("asg2"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("qa"), ResourceId: aws.String("asg3")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg3")},
									},
									AutoScalingGroupName: aws.String("asg3"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("qa"), ResourceId: aws.String("asg4")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg4")},
									},
									AutoScalingGroupName: aws.String("asg4"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Test opt-out mode",
			// Run on all groups except for those tagged with spot-enabled=false
			want: []string{"asg2", "asg3"},
			tregion: &region{
				tagsToFilterASGsBy: []Tag{{Key: "spot-enabled", Value: "false"}},
				conf:               &Config{TagFilteringMode: "opt-out"},
				services: connections{
					autoScaling: mockASG{
						dasgo: &autoscaling.DescribeAutoScalingGroupsOutput{
							AutoScalingGroups: []*autoscaling.Group{
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg1")},
										{Key: aws.String("spot-enabled"), Value: aws.String("false"), ResourceId: aws.String("asg1")},
									},
									AutoScalingGroupName: aws.String("asg1"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg2")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg2")},
									},
									AutoScalingGroupName: aws.String("asg2"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("qa"), ResourceId: aws.String("asg3")},
									},
									AutoScalingGroupName: aws.String("asg3"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("qa"), ResourceId: aws.String("asg4")},
										{Key: aws.String("spot-enabled"), Value: aws.String("false"), ResourceId: aws.String("asg4")},
									},
									AutoScalingGroupName: aws.String("asg4"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Test opt-out mode with multiple tag filters",
			// Run on all groups except for those tagged with spot-enabled=false and
			// environment=dev, regardless of other tags that may be set
			want: []string{"asg2", "asg3", "asg4"},
			tregion: &region{
				tagsToFilterASGsBy: []Tag{
					{Key: "spot-enabled", Value: "false"},
					{Key: "environment", Value: "dev"},
				},
				conf: &Config{TagFilteringMode: "opt-out"},
				services: connections{
					autoScaling: mockASG{
						dasgo: &autoscaling.DescribeAutoScalingGroupsOutput{
							AutoScalingGroups: []*autoscaling.Group{
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("spot-enabled"), Value: aws.String("false"), ResourceId: aws.String("asg1")},
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg1")},
										{Key: aws.String("team"), Value: aws.String("awesome"), ResourceId: aws.String("asg1")},
									},
									AutoScalingGroupName: aws.String("asg1"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg2")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg2")},
										{Key: aws.String("team"), Value: aws.String("awesome"), ResourceId: aws.String("asg2")},
									},
									AutoScalingGroupName: aws.String("asg2"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("spot-enabled"), Value: aws.String("false"), ResourceId: aws.String("asg3")},
										{Key: aws.String("environment"), Value: aws.String("qa"), ResourceId: aws.String("asg3")},
										{Key: aws.String("team"), Value: aws.String("awesome"), ResourceId: aws.String("asg3")},
									},
									AutoScalingGroupName: aws.String("asg3"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("qa"), ResourceId: aws.String("asg4")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg4")},
										{Key: aws.String("team"), Value: aws.String("awesome"), ResourceId: aws.String("asg4")},
									},
									AutoScalingGroupName: aws.String("asg4"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Test with two filters",
			want: []string{"asg3", "asg4"},
			tregion: &region{
				tagsToFilterASGsBy: []Tag{{Key: "spot-enabled", Value: "true"}, {Key: "environment", Value: "qa"}},
				conf:               &Config{},
				services: connections{
					autoScaling: mockASG{
						dasgo: &autoscaling.DescribeAutoScalingGroupsOutput{
							AutoScalingGroups: []*autoscaling.Group{
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg1")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg1")},
									},
									AutoScalingGroupName: aws.String("asg1"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg2")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg2")},
									},
									AutoScalingGroupName: aws.String("asg2"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("qa"), ResourceId: aws.String("asg3")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg3")},
									},
									AutoScalingGroupName: aws.String("asg3"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("qa"), ResourceId: aws.String("asg4")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg4")},
									},
									AutoScalingGroupName: aws.String("asg4"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Test with multiple secondary filter",
			want: []string{"asg4"},
			tregion: &region{
				tagsToFilterASGsBy: []Tag{
					{Key: "spot-enabled", Value: "true"},
					{Key: "environment", Value: "qa"},
					{Key: "team", Value: "interactive"},
				},
				conf: &Config{},
				services: connections{
					autoScaling: mockASG{
						dasgo: &autoscaling.DescribeAutoScalingGroupsOutput{
							AutoScalingGroups: []*autoscaling.Group{
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg1")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg1")},
									},
									AutoScalingGroupName: aws.String("asg1"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg2")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg2")},
									},
									AutoScalingGroupName: aws.String("asg2"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("qa"), ResourceId: aws.String("asg3")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg3")},
									},
									AutoScalingGroupName: aws.String("asg3"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("qa"), ResourceId: aws.String("asg4")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg4")},
										{Key: aws.String("team"), Value: aws.String("interactive"), ResourceId: aws.String("asg4")},
									},
									AutoScalingGroupName: aws.String("asg4"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Test with multiple secondary filters with glob expression",
			tregion: &region{
				tagsToFilterASGsBy: []Tag{
					{Key: "spot-enabled", Value: "true"},
					{Key: "environment", Value: "sandbox*"},
					{Key: "team", Value: "interactive"},
				},
				conf: &Config{},
				services: connections{
					autoScaling: mockASG{
						dasgo: &autoscaling.DescribeAutoScalingGroupsOutput{
							AutoScalingGroups: []*autoscaling.Group{
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("customer1-dev"), ResourceId: aws.String("asg1")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg1")},
									},
									AutoScalingGroupName: aws.String("asg1"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("sandbox-dev"), ResourceId: aws.String("asg2")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg2")},
										{Key: aws.String("team"), Value: aws.String("interactive"), ResourceId: aws.String("asg2")},
									},
									AutoScalingGroupName: aws.String("asg2"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("qa"), ResourceId: aws.String("asg3")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg3")},
									},
									AutoScalingGroupName: aws.String("asg3"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("sandbox-qa"), ResourceId: aws.String("asg4")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg4")},
										{Key: aws.String("team"), Value: aws.String("interactive"), ResourceId: aws.String("asg4")},
									},
									AutoScalingGroupName: aws.String("asg4"),
								},
							},
						},
					},
				},
			},
			want: []string{"asg2", "asg4"},
		},
		{
			name: "Test filters with invalid glob expression",
			tregion: &region{
				tagsToFilterASGsBy: []Tag{
					{Key: "spot-enabled", Value: "true"},
					{Key: "environment", Value: "($"},
					{Key: "team", Value: "interactive"},
				},
				conf: &Config{},
				services: connections{
					autoScaling: mockASG{
						dasgo: &autoscaling.DescribeAutoScalingGroupsOutput{
							AutoScalingGroups: []*autoscaling.Group{
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("customer1-dev"), ResourceId: aws.String("asg1")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg1")},
									},
									AutoScalingGroupName: aws.String("asg1"),
								},
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg2")},
										{Key: aws.String("team"), Value: aws.String("interactive"), ResourceId: aws.String("asg2")},
									},
									AutoScalingGroupName: aws.String("asg2"),
								},
							},
						},
					},
				},
			},
			want: nullSlice,
		},
		{
			name: "Test skipping execution against mixed groups",
			want: []string{"asg1"},
			tregion: &region{
				tagsToFilterASGsBy: []Tag{{Key: "spot-enabled", Value: "true"}},
				conf:               &Config{},
				services: connections{
					autoScaling: mockASG{
						dasgo: &autoscaling.DescribeAutoScalingGroupsOutput{
							AutoScalingGroups: []*autoscaling.Group{
								{
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg1")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg1")},
									},
									AutoScalingGroupName: aws.String("asg1"),
								},
								{
									MixedInstancesPolicy: &autoscaling.MixedInstancesPolicy{},
									Tags: []*autoscaling.TagDescription{
										{Key: aws.String("environment"), Value: aws.String("dev"), ResourceId: aws.String("asg2")},
										{Key: aws.String("spot-enabled"), Value: aws.String("true"), ResourceId: aws.String("asg2")},
									},
									AutoScalingGroupName: aws.String("asg2"),
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.tregion
			r.services.cloudFormation = mockCloudFormation{
				dso: &cloudformation.DescribeStacksOutput{
					Stacks: []*cloudformation.Stack{
						{
							StackName:   &stackName,
							StackStatus: &stackStatus,
						},
					},
				},
			}
			r.scanForEnabledAutoScalingGroups()
			var asgNames []string
			for _, name := range r.enabledASGs {
				asgNames = append(asgNames, name.name)
			}
			if !reflect.DeepEqual(tt.want, asgNames) {
				t.Errorf("region.scanForEnabledAutoScalingGroups() = %v, want %v", asgNames, tt.want)
			}
		})
	}
}

func TestIsStackUpdating(t *testing.T) {
	stackName := "dummyStackName"

	tests := []struct {
		name   string
		region *region
		want   bool
	}{
		{
			name: "Stack is not updating",
			region: &region{
				services: connections{
					cloudFormation: mockCloudFormation{
						dso: &cloudformation.DescribeStacksOutput{
							Stacks: []*cloudformation.Stack{
								{
									StackName:   &stackName,
									StackStatus: aws.String("UPDATE_COMPLETE"),
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "Stack is updating",
			region: &region{
				services: connections{
					cloudFormation: mockCloudFormation{
						dso: &cloudformation.DescribeStacksOutput{
							Stacks: []*cloudformation.Stack{
								{
									StackName:   &stackName,
									StackStatus: aws.String("UPDATE_IN_PROGRESS"),
								},
							},
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := tt.region.isStackUpdating(&stackName); got != tt.want {
				t.Errorf("Error in isStackUpdating: expected %v actual %v", tt.want, got)
			}
		})
	}
}

func Test_region_scanInstances(t *testing.T) {

	tests := []struct {
		name          string
		regionInfo    *region
		wantErr       bool
		wantInstances instances
	}{
		{
			name: "region with a single instance",
			regionInfo: &region{
				name: "us-east-1",
				conf: &Config{
					AutoScalingConfig: AutoScalingConfig{
						MinOnDemandNumber: 2,
					},
				},
				services: connections{
					ec2: mockEC2{
						diperr: nil,
						dio: &ec2.DescribeInstancesOutput{
							Reservations: []*ec2.Reservation{
								{
									Instances: []*ec2.Instance{
										{
											InstanceId:   aws.String("id-1"),
											InstanceType: aws.String("typeX"),
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
			wantInstances: makeInstancesWithCatalog(
				instanceMap{
					"id-1": {
						Instance: &ec2.Instance{
							InstanceId:   aws.String("id-1"),
							InstanceType: aws.String("typeX"),
						},
					},
				},
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.regionInfo
			err := r.scanInstances()

			if (err != nil) != tt.wantErr {
				t.Errorf("region.scanInstances() error = %v, wantErr %v", err, tt.wantErr)
			}

			for inst := range r.instances.instances() {
				wantedInstance := tt.wantInstances.get(*inst.InstanceId).Instance

				if !reflect.DeepEqual(inst.Instance, wantedInstance) {
					t.Errorf("region.scanInstances() \nreceived instance data: \n %+v\nexpected: \n %+v",
						spew.Sdump(inst.Instance), spew.Sdump(wantedInstance))

				}
			}

		})
	}
}

func Test_region_processDescribeInstancesPage(t *testing.T) {
	type regionFields struct {
		name      string
		instances instances
	}
	type args struct {
		page     *ec2.DescribeInstancesOutput
		lastPage bool
	}
	tests := []struct {
		name          string
		regionFields  regionFields
		args          args
		want          bool
		wantInstances instances
	}{
		{
			name: "region with a single instance",
			regionFields: regionFields{
				name:      "us-east-1",
				instances: makeInstancesWithCatalog(instanceMap{}),
			},
			args: args{
				page: &ec2.DescribeInstancesOutput{
					Reservations: []*ec2.Reservation{
						{
							Instances: []*ec2.Instance{{
								InstanceId:   aws.String("id-1"),
								InstanceType: aws.String("typeX"),
							},
								{
									InstanceId:   aws.String("id-2"),
									InstanceType: aws.String("typeY"),
								},
							},
						},
					},
				},
				lastPage: true,
			},
			want: true,
			wantInstances: makeInstancesWithCatalog(
				instanceMap{
					"id-1": {
						Instance: &ec2.Instance{
							InstanceId:   aws.String("id-1"),
							InstanceType: aws.String("typeX"),
						},
					},
					"id-2": {
						Instance: &ec2.Instance{
							InstanceId:   aws.String("id-2"),
							InstanceType: aws.String("typeY"),
						},
					},
				},
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &region{
				name:      tt.regionFields.name,
				instances: tt.regionFields.instances,
			}
			if got := r.processDescribeInstancesPage(tt.args.page, tt.args.lastPage); got != tt.want {
				t.Errorf("region.processDescribeInstancesPage() = %v, want %v", got, tt.want)
			}
		})
	}
}
