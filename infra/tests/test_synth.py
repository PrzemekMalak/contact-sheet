import aws_cdk as cdk
from aws_cdk.assertions import Template
from photo_infra.stack import OriginalsStack

def _template(**kwargs) -> Template:
    app = cdk.App()
    stack = OriginalsStack(app, "TestStack",
                            env=cdk.Environment(region="us-east-1"), **kwargs)
    return Template.from_stack(stack)

def test_stack_synthesizes():
    tmpl = _template()
    assert tmpl.to_json() is not None
