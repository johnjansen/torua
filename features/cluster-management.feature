Feature: Cluster Management
  As a cluster administrator
  I want to manage nodes and shards in the distributed system
  So that I can maintain a healthy and balanced cluster

  Background:
    Given a coordinator is running on port 8080
    And the cluster starts with 2 nodes

  Scenario: Initial cluster formation
    When I query the cluster status
    Then the coordinator should be healthy
    And there should be 2 registered nodes
    And all nodes should be marked as healthy
    And shards should be evenly distributed

  Scenario: Node health monitoring
    Given node "n1" is healthy
    When node "n1" stops responding to health checks
    Then within 10 seconds the coordinator should mark node "n1" as unhealthy
    And the coordinator should attempt to redistribute shards from node "n1"

  Scenario: Node graceful shutdown
    Given node "n1" has 2 shards assigned
    When node "n1" initiates graceful shutdown
    Then node "n1" should notify the coordinator
    And the coordinator should reassign node "n1"'s shards to other nodes
    And node "n1" should wait for confirmation before shutting down
    And no data should be lost during the transition

  Scenario: New node auto-registration
    When a new node "n3" starts on port 8083
    Then node "n3" should automatically register with the coordinator
    And the coordinator should add node "n3" to the cluster
    And node "n3" should appear in the nodes list within 5 seconds
    And the coordinator should consider rebalancing shards

  Scenario: Shard rebalancing after node addition
    Given the cluster has 4 shards distributed across 2 nodes
    When a new node "n3" joins the cluster
    Then the coordinator should detect the imbalance
    And the coordinator should redistribute shards for even distribution
    And each node should have approximately the same number of shards
    And data accessibility should be maintained during rebalancing

  Scenario: Coordinator failover detection
    Given a backup coordinator is configured
    When the primary coordinator becomes unresponsive
    Then nodes should detect the coordinator failure
    And nodes should attempt to connect to the backup coordinator
    And the cluster should continue operating with the backup coordinator

  Scenario: Split-brain prevention
    Given the cluster has 3 nodes
    When network partition splits node "n3" from the coordinator
    Then node "n3" should stop accepting write requests
    And the coordinator should mark node "n3" as unreachable
    And the majority partition should continue operating
    And node "n3" should attempt to rejoin when connectivity is restored

  Scenario: Node capacity limits
    Given node "n1" has a maximum shard capacity of 4
    And node "n1" already has 4 shards assigned
    When the coordinator needs to assign a new shard
    Then the coordinator should not assign it to node "n1"
    And the coordinator should choose a node with available capacity

  Scenario: Cluster information API
    When I GET "/cluster/info" from the coordinator
    Then the response should include:
      | field               | description                  |
      | cluster_id          | Unique cluster identifier    |
      | coordinator_version | Version of coordinator       |
      | total_nodes         | Number of registered nodes   |
      | healthy_nodes       | Number of healthy nodes      |
      | total_shards        | Total number of shards       |
      | assigned_shards     | Number of assigned shards    |
      | cluster_state       | Overall cluster health state |
      | uptime              | Cluster uptime in seconds    |

  Scenario: Node information API
    When I GET "/nodes/{node_id}/info" from the coordinator
    Then the response should include:
      | field              | description                   |
      | node_id            | Unique node identifier        |
      | address            | Node network address          |
      | status             | Current node status           |
      | shard_count        | Number of shards on this node |
      | last_heartbeat     | Timestamp of last heartbeat   |
      | uptime             | Node uptime in seconds        |
      | available_capacity | Remaining shard capacity      |

  Scenario: Bulk node operations
    Given I have 3 healthy nodes in the cluster
    When I POST "/nodes/maintenance" with node list ["n1", "n2"]
    Then nodes "n1" and "n2" should enter maintenance mode
    And their shards should be redistributed to node "n3"
    And the nodes should stop accepting new shards
    But the nodes should continue serving existing data

  Scenario: Cluster-wide configuration update
    When I PUT "/cluster/config" with new configuration
    Then the coordinator should validate the configuration
    And the coordinator should broadcast the update to all nodes
    And all nodes should acknowledge the configuration change
    And the new configuration should take effect cluster-wide

  Scenario: Emergency shard evacuation
    Given node "n1" is experiencing hardware issues
    When I POST "/nodes/n1/evacuate"
    Then the coordinator should immediately start moving shards off node "n1"
    And the evacuation should prioritize primary shards
    And the coordinator should report evacuation progress
    And node "n1" should be marked as "evacuating"

  Scenario: Automatic failure recovery
    Given node "n2" has been offline for 5 minutes
    And node "n2"'s shards have been reassigned
    When node "n2" comes back online
    Then node "n2" should re-register with the coordinator
    And the coordinator should mark node "n2" as available
    But the coordinator should not automatically reassign shards back
    And manual rebalancing should be required

  Scenario: Monitoring metrics exposure
    When I GET "/metrics" from the coordinator
    Then the response should be in Prometheus format
    And it should include metrics for:
      | metric_type        | description                |
      | node_count         | Number of nodes by status  |
      | shard_distribution | Shards per node            |
      | request_latency    | Request processing times   |
      | error_rate         | Error rates by type        |
      | heap_usage         | Memory usage per component |
      | network_traffic    | Bytes sent/received        |

  Scenario: Rolling cluster upgrade
    Given all nodes are running version "1.0.0"
    When I initiate a rolling upgrade to version "1.1.0"
    Then the coordinator should upgrade nodes one at a time
    And each node should gracefully transfer its shards before upgrading
    And the cluster should remain available throughout the upgrade
    And all nodes should report version "1.1.0" when complete

  @slow
  Scenario: Load-based shard rebalancing
    Given the cluster is experiencing uneven load distribution
    When node "n1" consistently shows 80% higher load than other nodes
    Then the coordinator should detect the load imbalance
    And the coordinator should identify hot shards on node "n1"
    And the coordinator should redistribute hot shards to less loaded nodes
    And the load should become more evenly distributed

  @integration
  Scenario: Multi-coordinator consensus
    Given 3 coordinator instances are running for high availability
    When the leader coordinator receives a cluster state change
    Then the leader should replicate the change to follower coordinators
    And all coordinators should reach consensus on the new state
    And any coordinator should be able to serve read requests
    But only the leader should handle write operations
