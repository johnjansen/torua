"""
Common step definitions shared across multiple feature files.

These steps are used by multiple features and are defined here to avoid duplication.
"""

import requests
from behave import given, when, then
from hamcrest import assert_that, equal_to


def make_request(method: str, url: str, **kwargs) -> requests.Response:
    """Make an HTTP request with default timeout."""
    kwargs.setdefault('timeout', 5)
    return requests.request(method, url, **kwargs)


# Common Given steps

@given('a coordinator is running on port {port:d}')
def step_coordinator_running(context, port):
    """Verify the coordinator is running on the specified port."""
    # The coordinator should already be started by environment.py
    coordinator_url = f"http://localhost:{port}"
    context.coordinator_url = coordinator_url

    # Verify it's healthy
    response = make_request('GET', f"{coordinator_url}/health")
    assert_that(response.status_code, equal_to(200))

    # Store coordinator info in context
    context.test.coordinator_url = coordinator_url


# Common When steps

@when('I GET "{endpoint}" from the coordinator')
def step_get_coordinator_endpoint(context, endpoint):
    """Make a GET request to a coordinator endpoint."""
    response = make_request('GET', f"{context.coordinator_url}{endpoint}")
    context.last_response = response
    context.last_endpoint = endpoint


# Common Then steps

@then('the response status should be {status_code:d}')
def step_verify_status_code(context, status_code):
    """Verify the response status code."""
    assert_that(context.last_response.status_code, equal_to(status_code))
