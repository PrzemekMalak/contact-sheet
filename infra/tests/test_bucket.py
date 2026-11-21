import aws_cdk as cdk
from aws_cdk.assertions import Template, Match
from photo_infra.stack import OriginalsStack

def _template(**kwargs) -> Template:
    app = cdk.App()
    stack = OriginalsStack(app, "TestStack",
                            env=cdk.Environment(region="us-east-1"), **kwargs)
    return Template.from_stack(stack)

def test_bucket_blocks_all_public_access():
    _template().has_resource_properties("AWS::S3::Bucket", {
        "PublicAccessBlockConfiguration": {
            "BlockPublicAcls": True, "BlockPublicPolicy": True,
            "IgnorePublicAcls": True, "RestrictPublicBuckets": True,
        }
    })

def test_bucket_is_encrypted():
    _template().has_resource_properties("AWS::S3::Bucket", {
        "BucketEncryption": Match.object_like({
            "ServerSideEncryptionConfiguration": Match.any_value()
        })
    })

def test_bucket_retained_on_delete():
    _template().has_resource("AWS::S3::Bucket", {"DeletionPolicy": "Retain"})

def test_bucket_enforces_ssl():
    # enforce_ssl adds a bucket policy denying non-TLS access
    _template().has_resource_properties("AWS::S3::BucketPolicy", {
        "PolicyDocument": Match.object_like({
            "Statement": Match.array_with([
                Match.object_like({
                    "Effect": "Deny",
                    "Condition": {"Bool": {"aws:SecureTransport": "false"}},
                })
            ])
        })
    })
