# Feature: Distributed Key-Value Storage
# As a client application
# I want to store and retrieve data through a single coordinator endpoint
# So that I don't need to know about sharding or node distribution
Feature: Distributed Storage System

  Background:
    Given a coordinator is running on port 8080
    And node "n1" is running on port 8081
    And node "n2" is running on port 8082
    And the system has 4 shards configured
    And shards are distributed across nodes

  Scenario: Store and retrieve a simple value
    When I PUT "Hello World" to key "greeting"
    Then the response status should be 204
    When I GET the key "greeting"
    Then the response status should be 200
    And the response body should be "Hello World"

  Scenario: Update an existing value
    Given the key "counter" contains "1"
    When I PUT "2" to key "counter"
    Then the response status should be 204
    When I GET the key "counter"
    Then the response body should be "2"
    And the old value "1" should no longer exist

  Scenario: Delete a value
    Given the key "temp" contains "temporary data"
    When I DELETE the key "temp"
    Then the response status should be 204
    When I GET the key "temp"
    Then the response status should be 404

  Scenario: Retrieve non-existent key
    When I GET the key "does-not-exist"
    Then the response status should be 404

  Scenario: Keys are distributed across shards
    When I PUT "value1" to key "key1"
    And I PUT "value2" to key "key2"
    And I PUT "value3" to key "key3"
    And I PUT "value4" to key "key4"
    Then the keys should be distributed across multiple shards
    And each key should be retrievable

  Scenario: Consistent routing for same key
    When I PUT "initial" to key "consistent-key"
    And I GET the key "consistent-key" 10 times
    Then all GET requests should return "initial"
    And all requests should route to the same shard

  Scenario: Node failure handling
    Given the key "important" contains "critical data"
    When node "n1" becomes unavailable
    And I GET the key "important"
    Then the response status should be 502 or 503
    # Note: Without replication, data on failed node is unavailable

  Scenario: New node joins cluster
    When node "n3" registers with the coordinator on port 8083
    Then the coordinator should recognize 3 nodes
    And new shards can be assigned to node "n3"
    And existing data remains accessible

  Scenario: Transparent sharding
    When I PUT "data" to key "user:123:profile"
    Then I should not need to specify which shard to use
    And I should not need to know which node stores the data
    When I GET the key "user:123:profile"
    Then the response body should be "data"

  Scenario: Large value storage
    Given a value of 1MB size
    When I PUT the large value to key "large-file"
    Then the response status should be 204
    When I GET the key "large-file"
    Then the response should match the original large value

  Scenario: Concurrent operations
    When 10 clients simultaneously PUT different values to different keys
    Then all PUT operations should succeed
    When the same 10 clients GET their respective keys
    Then each client should receive their correct value

  Scenario: Shard information visibility
    When I GET "/shards" from the coordinator
    Then the response should list all shard assignments
    And each shard should show its assigned node
    And the total number of shards should be 4

  Scenario: Node information visibility
    When I GET "/nodes" from the coordinator
    Then the response should list all registered nodes
    And each node should show its address
    When I GET "/info" from node "n1"
    Then the response should show which shards it owns

  Scenario Outline: Storage operations with various key patterns
    When I PUT "<value>" to key "<key>"
    Then the response status should be 204
    When I GET the key "<key>"
    Then the response body should be "<value>"

    Examples:
      | key                                         | value         |
      | simple                                      | text          |
      | user@example.com                            | email-data    |
      | path/to/resource                            | nested-data   |
      | key-with-spaces here                        | spaced-value  |
      | 数字                                        | unicode-value |
      | very:long:key:with:many:colons:and:segments | complex       |

  Scenario: Performance requirements
    Given 1000 keys are stored in the system
    When I GET a random key
    Then the response time should be less than 50ms
    When I PUT a new value
    Then the response time should be less than 50ms

  Scenario: Coordinator routes requests correctly
    Given I can trace the request path
    When I PUT "test" to key "traceable"
    Then the coordinator should:
      | action                    | details                   |
      | Calculate shard ID        | Using hash(key) % 4       |
      | Look up node for shard    | Find node assignment      |
      | Forward request to node   | PUT /shard/{id}/store/key |
      | Return response to client | Status 204                |
