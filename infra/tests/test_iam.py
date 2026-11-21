import aws_cdk as cdk
from aws_cdk.assertions import Template, Match
from photo_infra.stack import OriginalsStack

def _template(**kwargs) -> Template:
    app = cdk.App()
    stack = OriginalsStack(app, "TestStack",
                            env=cdk.Environment(region="us-east-1"), **kwargs)
    return Template.from_stack(stack)

def test_uploader_user_created_no_access_key():
    tmpl = _template()
    tmpl.resource_count_is("AWS::IAM::User", 1)
    # CDK must NOT create long-lived access keys in the template
    tmpl.resource_count_is("AWS::IAM::AccessKey", 0)

def test_policy_allows_put_and_delete_on_originals_only():
    _template().has_resource_properties("AWS::IAM::Policy", {
        "PolicyDocument": Match.object_like({
            "Statement": Match.array_with([
                Match.object_like({
                    "Action": Match.array_with(["s3:PutObject", "s3:DeleteObject"]),
                    "Effect": "Allow",
                    # Resource must be scoped to the originals/* prefix, not the
                    # whole bucket ARN and not "*". The CDK renders this as an
                    # Fn::Join of [GetAtt bucket Arn, "/originals/*"].
                    "Resource": Match.object_like({
                        "Fn::Join": Match.array_with([
                            Match.array_with([Match.string_like_regexp(r".*originals/\*$")])
                        ])
                    }),
                })
            ])
        })
    })

def test_policy_allows_listbucket():
    _template().has_resource_properties("AWS::IAM::Policy", {
        "PolicyDocument": Match.object_like({
            "Statement": Match.array_with([
                Match.object_like({"Action": "s3:ListBucket", "Effect": "Allow"})
            ])
        })
    })
