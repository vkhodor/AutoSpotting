#!/bin/bash

export ALLOWED_INSTANCE_TYPES="c6g.4xlarge, m6g.4xlarge, r6g.4xlarge, c6gd.4xlarge"
export BIDDING_POLICY="normal"
export INSTANCE_TERMINATION_METHO="autoscaling"
export LICENSE="evaluation"
export REGIONS="us-east-2, ap-northeast-1"

./AutoSpotting

