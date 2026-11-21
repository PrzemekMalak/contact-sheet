import sys, pathlib
import aws_cdk as cdk
from aws_cdk.assertions import Template

INFRA = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(INFRA))

HOSTING = {
    "repository": "https://github.com/user/photo-site",
    "github_token_secret_arn": "arn:aws:secretsmanager:us-east-1:111122223333:secret:gh-abc",
}

def _app(context: dict) -> cdk.App:
    app = cdk.App(context=context)
    import build_app
    build_app.build(app)
    return app

def _stacks(context: dict) -> dict[str, cdk.Stack]:
    return {c.stack_name: c for c in _app(context).node.children if isinstance(c, cdk.Stack)}

def test_hosting_absent_without_repository():
    names = _stacks({})
    assert "PhotoSiteStack" in names
    assert "PhotoSiteHostingStack" not in names

def test_hosting_present_with_repository():
    assert "PhotoSiteHostingStack" in _stacks(HOSTING)

def test_site_name_and_stack_prefix_from_context():
    stacks = _stacks({**HOSTING, "site_name": "example.com", "stack_prefix": "Example"})
    assert set(stacks) == {"ExampleStack", "ExampleHostingStack"}
    Template.from_stack(stacks["ExampleStack"]).has_resource_properties(
        "AWS::IAM::User", {"UserName": "photo-uploader"})
    Template.from_stack(stacks["ExampleStack"]).has_resource_properties(
        "AWS::CloudFront::Distribution",
        {"DistributionConfig": {"Comment": "example.com originals CDN"}})
    Template.from_stack(stacks["ExampleHostingStack"]).has_resource_properties(
        "AWS::Amplify::App", {"Name": "example-com"})

def test_committed_cdk_json_uses_generic_defaults():
    import json
    ctx = json.loads((INFRA / "cdk.json").read_text())["context"]
    stacks = _stacks({**ctx, **HOSTING})
    assert set(stacks) == {"PhotoSiteStack", "PhotoSiteHostingStack"}
    Template.from_stack(stacks["PhotoSiteStack"]).has_resource_properties(
        "AWS::IAM::User", {"UserName": "photo-uploader"})

def _price_class(stack: cdk.Stack) -> str:
    (dist,) = Template.from_stack(stack).find_resources("AWS::CloudFront::Distribution").values()
    return dist["Properties"]["DistributionConfig"]["PriceClass"]

def test_price_class_defaults_to_100():
    assert _price_class(_stacks({})["PhotoSiteStack"]) == "PriceClass_100"

def test_price_class_from_context():
    for value in ("PriceClass_200", "PriceClass_All"):
        assert _price_class(_stacks({"price_class": value})["PhotoSiteStack"]) == value

def test_unknown_price_class_is_rejected():
    import pytest
    with pytest.raises(ValueError, match="price_class"):
        _stacks({"price_class": "PriceClass_50"})

def _budget_limit(stack: cdk.Stack):
    (budget,) = Template.from_stack(stack).find_resources("AWS::Budgets::Budget").values()
    return budget["Properties"]["Budget"]["BudgetLimit"]["Amount"]

def test_no_budget_by_default():
    Template.from_stack(_stacks({})["PhotoSiteStack"]).resource_count_is("AWS::Budgets::Budget", 0)

def test_budget_from_context():
    stack = _stacks({"budget_email": "me@example.com"})["PhotoSiteStack"]
    assert _budget_limit(stack) == 5

def test_budget_limit_from_context_string():
    # -c values arrive as strings.
    stack = _stacks({"budget_email": "me@example.com", "budget_limit": "10"})["PhotoSiteStack"]
    assert _budget_limit(stack) == 10

def test_invalid_budget_limit_is_rejected():
    import pytest
    for bad in ("abc", "0", "-3"):
        with pytest.raises(ValueError, match="budget_limit"):
            _stacks({"budget_email": "me@example.com", "budget_limit": bad})
