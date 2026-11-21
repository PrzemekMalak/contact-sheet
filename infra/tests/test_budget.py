import aws_cdk as cdk
from aws_cdk.assertions import Template, Match
from photo_infra.stack import OriginalsStack

def _template(**kwargs) -> Template:
    app = cdk.App()
    stack = OriginalsStack(app, "TestStack",
                           env=cdk.Environment(region="us-east-1"), **kwargs)
    return Template.from_stack(stack)

def test_no_budget_without_email():
    _template().resource_count_is("AWS::Budgets::Budget", 0)

def test_budget_with_email_defaults_to_5_usd_monthly():
    tmpl = _template(budget_email="me@example.com")
    tmpl.resource_count_is("AWS::Budgets::Budget", 1)
    tmpl.has_resource_properties("AWS::Budgets::Budget", {
        "Budget": Match.object_like({
            "BudgetType": "COST",
            "TimeUnit": "MONTHLY",
            "BudgetLimit": {"Amount": 5, "Unit": "USD"},
        }),
    })

def test_budget_alerts_on_actual_80_and_forecast_100():
    tmpl = _template(budget_email="me@example.com")
    subscriber = [{"SubscriptionType": "EMAIL", "Address": "me@example.com"}]
    tmpl.has_resource_properties("AWS::Budgets::Budget", {
        "NotificationsWithSubscribers": Match.array_equals([
            {"Notification": {"NotificationType": "ACTUAL", "ComparisonOperator": "GREATER_THAN",
                              "Threshold": 80, "ThresholdType": "PERCENTAGE"},
             "Subscribers": subscriber},
            {"Notification": {"NotificationType": "FORECASTED", "ComparisonOperator": "GREATER_THAN",
                              "Threshold": 100, "ThresholdType": "PERCENTAGE"},
             "Subscribers": subscriber},
        ]),
    })

def test_budget_limit_is_configurable():
    _template(budget_email="me@example.com", budget_limit=12.5).has_resource_properties(
        "AWS::Budgets::Budget",
        {"Budget": Match.object_like({"BudgetLimit": {"Amount": 12.5, "Unit": "USD"}})})
