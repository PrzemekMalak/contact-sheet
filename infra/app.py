#!/usr/bin/env python3
import aws_cdk as cdk
from build_app import build

app = cdk.App()
build(app)
app.synth()
