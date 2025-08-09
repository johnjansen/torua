"""
Behave environment setup for Torua end-to-end testing.

This module handles the lifecycle of the test environment, including:
- Starting/stopping coordinator and nodes
- Managing test data
- Cleaning up resources
- Providing shared context between steps
"""

import os
import sys
import time
import json
import signal
import subprocess
import logging
import shutil
import tempfile
from pathlib import Path
from typing import Dict, List, Optional, Any
from dataclasses import dataclass, field
from contextlib import contextmanager

import requests
import psutil
from tenacity import retry, stop_after_delay, wait_fixed

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(name)s: %(message)s'
)
logger = logging.getLogger(__name__)


@dataclass
class ProcessInfo:
    """Information about a running process."""
    name: str
    process: Optional[subprocess.Popen] = None
    port: int = 0
    host: str = "localhost"
    pid: Optional[int] = None
    log_file: Optional[Path] = None

    @property
    def url(self) -> str:
        """Get the base URL for this process."""
        return f"http://{self.host}:{self.port}"

    def is_running(self) -> bool:
        """Check if the process is still running."""
        if self.process and self.process.poll() is None:
            return True
        if self.pid:
            try:
                process = psutil.Process(self.pid)
                return process.is_running()
            except psutil.NoSuchProcess:
                return False
        return False


@dataclass
class TestContext:
    """Context object to store test environment state."""
    coordinator: Optional[ProcessInfo] = None
    nodes: Dict[str, ProcessInfo] = field(default_factory=dict)
    goreman_process: Optional[subprocess.Popen] = None
    test_dir: Optional[Path] = None
    log_dir: Optional[Path] = None
    data_dir: Optional[Path] = None
    processes: List[subprocess.Popen] = field(default_factory=list)
    cleanup_on_exit: bool = True
    request_timeout: int = 5
    startup_timeout: int = 10
    suspended_pids: List[int] = field(default_factory=list)

    def get_coordinator_url(self) -> str:
        """Get the coordinator's base URL."""
        if not self.coordinator:
            raise RuntimeError("Coordinator not initialized")
        return self.coordinator.url

    def get_node_url(self, node_name: str) -> str:
        """Get a node's base URL by name."""
        if node_name not in self.nodes:
            raise RuntimeError(f"Node {node_name} not found")
        return self.nodes[node_name].url


class TorquaEnvironment:
    """Environment manager for Torua E2E tests."""

    def __init__(self, context):
        """Initialize the test environment."""
        self.context = context
        self.test_context = TestContext()

        # Get configuration from behave.ini
        self.coordinator_host = context.config.userdata.get('coordinator_host', 'localhost')
        self.coordinator_port = int(context.config.userdata.get('coordinator_port', 8080))
        self.node1_port = int(context.config.userdata.get('node1_port', 8081))
        self.node2_port = int(context.config.userdata.get('node2_port', 8082))
        self.node3_port = int(context.config.userdata.get('node3_port', 8083))
        self.startup_timeout = int(context.config.userdata.get('startup_timeout', 10))
        self.request_timeout = int(context.config.userdata.get('request_timeout', 5))
        self.cleanup_on_exit = context.config.userdata.getbool('cleanup_on_exit', True)

        # Store test context in behave context
        context.test = self.test_context
        context.test.request_timeout = self.request_timeout
        context.test.startup_timeout = self.startup_timeout
        context.test.cleanup_on_exit = self.cleanup_on_exit

    def setup_directories(self):
        """Set up test directories."""
        # Create temporary test directory
        self.test_context.test_dir = Path(tempfile.mkdtemp(prefix="torua_test_"))
        self.test_context.log_dir = self.test_context.test_dir / "logs"
        self.test_context.data_dir = self.test_context.test_dir / "data"

        # Create subdirectories
        self.test_context.log_dir.mkdir(parents=True, exist_ok=True)
        self.test_context.data_dir.mkdir(parents=True, exist_ok=True)

        logger.info(f"Test directory: {self.test_context.test_dir}")

    def cleanup_directories(self):
        """Clean up test directories."""
        if self.test_context.test_dir and self.test_context.cleanup_on_exit:
            try:
                shutil.rmtree(self.test_context.test_dir)
                logger.info(f"Cleaned up test directory: {self.test_context.test_dir}")
            except Exception as e:
                logger.warning(f"Failed to clean up test directory: {e}")

    def start_cluster_with_goreman(self):
        """Start the entire cluster using goreman or directly."""
        # Check if goreman is available
        try:
            subprocess.run(['which', 'goreman'], check=True, capture_output=True)
            use_goreman = True
        except (subprocess.CalledProcessError, FileNotFoundError):
            use_goreman = False
            logger.info("goreman not found, starting services directly")

        # Build services if needed
        self._build_service("coordinator")
        self._build_service("node")

        if use_goreman:
            logger.info("Starting cluster with goreman...")
            # Create a Procfile for this test run
            # Use 2 second health check interval for faster test execution (3 failures * 2s = 6s < 10s timeout)
            procfile_content = f"""coordinator: HEALTH_CHECK_INTERVAL=2s COORDINATOR_ADDR=:{self.coordinator_port} ./bin/coordinator
node1: NODE_ID=n1 NODE_LISTEN=:{self.node1_port} NODE_ADDR=http://{self.coordinator_host}:{self.node1_port} COORDINATOR_ADDR=http://{self.coordinator_host}:{self.coordinator_port} ./bin/node
node2: NODE_ID=n2 NODE_LISTEN=:{self.node2_port} NODE_ADDR=http://{self.coordinator_host}:{self.node2_port} COORDINATOR_ADDR=http://{self.coordinator_host}:{self.coordinator_port} ./bin/node
"""

            procfile_path = self.test_context.test_dir / "Procfile"
            with open(procfile_path, 'w') as f:
                f.write(procfile_content)

            # Start goreman
            log_file = self.test_context.log_dir / "goreman.log"
            with open(log_file, 'w') as log:
                process = subprocess.Popen(
                    ['goreman', '-f', str(procfile_path), 'start'],
                    cwd=self._get_project_root(),
                    stdout=log,
                    stderr=subprocess.STDOUT,
                    preexec_fn=os.setsid if sys.platform != 'win32' else None
                )
                self.test_context.goreman_process = process
                self.test_context.processes.append(process)
        else:
            logger.info("Starting services directly...")
            # Start coordinator first
            self._start_coordinator_directly()
            # Start nodes
            self._start_node_directly("n1", self.node1_port)
            self._start_node_directly("n2", self.node2_port)

        # Set up process info for coordinator
        self.test_context.coordinator = ProcessInfo(
            name="coordinator",
            port=self.coordinator_port,
            host=self.coordinator_host
        )

        # Set up process info for nodes
        self.test_context.nodes = {
            "n1": ProcessInfo(name="n1", port=self.node1_port, host=self.coordinator_host),
            "n2": ProcessInfo(name="n2", port=self.node2_port, host=self.coordinator_host)
        }

        # Wait for services to be ready
        self._wait_for_service(f"http://{self.coordinator_host}:{self.coordinator_port}/health")
        logger.info(f"Coordinator ready on port {self.coordinator_port}")

        self._wait_for_service(f"http://{self.coordinator_host}:{self.node1_port}/health")
        logger.info(f"Node n1 ready on port {self.node1_port}")

        self._wait_for_service(f"http://{self.coordinator_host}:{self.node2_port}/health")
        logger.info(f"Node n2 ready on port {self.node2_port}")

        # Wait for nodes to register
        self._wait_for_node_registration("n1")
        self._wait_for_node_registration("n2")
        logger.info("All nodes registered with coordinator")

    def _start_coordinator_directly(self):
        """Start the coordinator process directly without goreman."""
        logger.info(f"Starting coordinator on port {self.coordinator_port}...")

        coord = ProcessInfo(
            name="coordinator",
            port=self.coordinator_port,
            host=self.coordinator_host,
            log_file=self.test_context.log_dir / "coordinator.log"
        )

        env = os.environ.copy()
        env['COORDINATOR_ADDR'] = f':{self.coordinator_port}'
        env['HEALTH_CHECK_INTERVAL'] = '2s'

        with open(coord.log_file, 'w') as log:
            process = subprocess.Popen(
                ['./bin/coordinator'],
                cwd=self._get_project_root(),
                env=env,
                stdout=log,
                stderr=subprocess.STDOUT,
                preexec_fn=os.setsid if sys.platform != 'win32' else None
            )
            coord.process = process
            coord.pid = process.pid
            self.test_context.processes.append(process)
            self.test_context.coordinator = coord

        # Wait for coordinator to be ready
        self._wait_for_service(coord.url + "/health")
        logger.info(f"Coordinator started on port {self.coordinator_port}")

    def _start_node_directly(self, node_id: str, port: int):
        """Start a node process directly without goreman."""
        logger.info(f"Starting node {node_id} on port {port}...")

        node = ProcessInfo(
            name=node_id,
            port=port,
            host=self.coordinator_host,
            log_file=self.test_context.log_dir / f"{node_id}.log"
        )

        env = os.environ.copy()
        env['NODE_ID'] = node_id
        env['NODE_LISTEN'] = f':{port}'
        env['NODE_ADDR'] = f'http://{self.coordinator_host}:{port}'
        env['COORDINATOR_ADDR'] = f'http://{self.coordinator_host}:{self.coordinator_port}'
        env['HEALTH_CHECK_INTERVAL'] = '2s'

        # Create node data directory
        node_data_dir = self.test_context.data_dir / node_id
        node_data_dir.mkdir(parents=True, exist_ok=True)
        env['DATA_DIR'] = str(node_data_dir)

        with open(node.log_file, 'w') as log:
            process = subprocess.Popen(
                ['./bin/node'],
                cwd=self._get_project_root(),
                env=env,
                stdout=log,
                stderr=subprocess.STDOUT,
                preexec_fn=os.setsid if sys.platform != 'win32' else None
            )
            node.process = process
            node.pid = process.pid
            self.test_context.processes.append(process)
            self.test_context.nodes[node_id] = node

        # Wait for node to be ready
        self._wait_for_service(node.url + "/health")
        logger.info(f"Node {node_id} started on port {port}")

        # Wait for node to register with coordinator
        self._wait_for_node_registration(node_id)

    def start_coordinator(self):
        """Start the coordinator process (deprecated - use start_cluster_with_goreman)."""
        # This method is kept for backward compatibility but delegates to cluster start
        pass

    def start_node(self, node_name: str, port: int):
        """Start a node process."""
        logger.info(f"Starting node {node_name} on port {port}...")

        # Build the node if needed
        self._build_service("node")

        # Set up node process
        node = ProcessInfo(
            name=node_name,
            port=port,
            host=self.coordinator_host,
            log_file=self.test_context.log_dir / f"{node_name}.log"
        )

        # Start node
        env = os.environ.copy()
        env['NODE_ID'] = node_name
        env['NODE_LISTEN'] = f':{port}'
        env['NODE_ADDR'] = f'http://{self.coordinator_host}:{port}'
        env['COORDINATOR_ADDR'] = self.test_context.coordinator.url
        env['DATA_DIR'] = str(self.test_context.data_dir / node_name)
        env['HEALTH_CHECK_INTERVAL'] = '2s'

        # Create node data directory
        node_data_dir = self.test_context.data_dir / node_name
        node_data_dir.mkdir(parents=True, exist_ok=True)

        with open(node.log_file, 'w') as log:
            process = subprocess.Popen(
                ['./bin/node'],
                cwd=self._get_project_root(),
                env=env,
                stdout=log,
                stderr=subprocess.STDOUT,
                preexec_fn=os.setsid if sys.platform != 'win32' else None
            )
            node.process = process
            node.pid = process.pid
            self.test_context.processes.append(process)
            self.test_context.nodes[node_name] = node

        # Wait for node to be ready
        self._wait_for_service(node.url + "/health")
        logger.info(f"Node {node_name} started on port {port}")

        # Wait for node to register with coordinator
        self._wait_for_node_registration(node_name)

    def stop_all_processes(self):
        """Stop all running processes."""
        logger.info("Stopping all processes...")

        # Clean up any suspended processes first
        if hasattr(self, 'test_context') and hasattr(self.test_context, 'suspended_pids'):
            for pid in self.test_context.suspended_pids:
                try:
                    os.kill(pid, signal.SIGKILL)
                    logger.info(f"Killed suspended process {pid}")
                except (ProcessLookupError, OSError):
                    pass  # Process already dead
            self.test_context.suspended_pids.clear()

        # If using goreman, stop it first
        if hasattr(self.test_context, 'goreman_process') and self.test_context.goreman_process:
            logger.info("Stopping goreman...")
            try:
                if sys.platform != 'win32':
                    os.killpg(os.getpgid(self.test_context.goreman_process.pid), signal.SIGTERM)
                else:
                    self.test_context.goreman_process.terminate()
                self.test_context.goreman_process.wait(timeout=5)
                logger.info("Goreman stopped")
            except Exception as e:
                logger.warning(f"Failed to stop goreman gracefully: {e}")
                try:
                    self.test_context.goreman_process.kill()
                except:
                    pass

        # Stop individual nodes if they exist
        if hasattr(self.test_context, 'nodes'):
            for node_name, node in self.test_context.nodes.items():
                if node.process:
                    self._stop_process(node, f"node {node_name}")

        # Stop coordinator
        if self.test_context.coordinator and self.test_context.coordinator.process:
            self._stop_process(self.test_context.coordinator, "coordinator")

        # Clean up any remaining processes
        for process in self.test_context.processes:
            if process.poll() is None:
                try:
                    if sys.platform != 'win32':
                        os.killpg(os.getpgid(process.pid), signal.SIGTERM)
                    else:
                        process.terminate()
                    process.wait(timeout=5)
                except Exception as e:
                    logger.warning(f"Failed to terminate process {process.pid}: {e}")
                    try:
                        process.kill()
                    except:
                        pass

    def _stop_process(self, process_info: ProcessInfo, name: str):
        """Stop a specific process."""
        if not process_info or not process_info.process:
            return

        if process_info.process.poll() is None:
            logger.info(f"Stopping {name}...")
            try:
                process_info.process.terminate()
                process_info.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                logger.warning(f"{name} didn't stop gracefully, killing...")
                process_info.process.kill()
                process_info.process.wait()
            logger.info(f"{name} stopped")

    def _build_service(self, service: str):
        """Build a service if needed."""
        project_root = self._get_project_root()
        binary_path = project_root / "bin" / service

        if not binary_path.exists():
            logger.info(f"Building {service}...")
            result = subprocess.run(
                ['make', f'build-{service}'],
                cwd=project_root,
                capture_output=True,
                text=True
            )
            if result.returncode != 0:
                raise RuntimeError(f"Failed to build {service}: {result.stderr}")
            logger.info(f"{service} built successfully")

    def _get_project_root(self) -> Path:
        """Get the project root directory."""
        # Assuming environment.py is in torua/features/
        return Path(__file__).parent.parent

    @retry(stop=stop_after_delay(30), wait=wait_fixed(0.5))
    def _wait_for_service(self, url: str):
        """Wait for a service to be ready."""
        response = requests.get(url, timeout=1)
        response.raise_for_status()
        logger.debug(f"Service at {url} is ready")

    @retry(stop=stop_after_delay(10), wait=wait_fixed(0.5))
    def _wait_for_node_registration(self, node_name: str):
        """Wait for a node to register with the coordinator."""
        try:
            url = f"http://{self.coordinator_host}:{self.coordinator_port}/nodes"
            response = requests.get(url, timeout=1)
            response.raise_for_status()
            data = response.json()

            # Extract nodes list from response
            nodes = data.get('nodes', []) if isinstance(data, dict) else data

            logger.debug(f"Checking registration for {node_name}, got nodes: {nodes}")

            # Check if node is registered by ID
            for node in nodes:
                if node.get('id') == node_name:
                    logger.debug(f"Node {node_name} is registered")
                    return

            # Also check by port if we have node info in test context
            if hasattr(self, 'test_context') and hasattr(self.test_context, 'nodes'):
                if node_name in self.test_context.nodes:
                    node_port = self.test_context.nodes[node_name].port
                    logger.debug(f"Looking for node with port {node_port}")
                    for node in nodes:
                        node_addr = node.get('addr', '')
                        if node_addr.endswith(str(node_port)):
                            logger.debug(f"Node {node_name} is registered (matched by port)")
                            return

            raise RuntimeError(f"Node {node_name} not registered yet. Current nodes: {nodes}")
        except Exception as e:
            logger.error(f"Error checking node registration: {type(e).__name__}: {e}")
            raise


def before_all(context):
    """
    Set up the test environment before all tests.

    This function is called once before any tests are run.
    """
    logger.info("Setting up Torua test environment...")

    # Create environment manager
    env_manager = TorquaEnvironment(context)
    context.env_manager = env_manager

    # Set up directories
    env_manager.setup_directories()

    # Start services using goreman
    try:
        env_manager.start_cluster_with_goreman()
        logger.info("Test environment ready")
    except Exception as e:
        logger.error(f"Failed to set up test environment: {e}")
        env_manager.stop_all_processes()
        env_manager.cleanup_directories()
        raise


def after_all(context):
    """
    Tear down the test environment after all tests.

    This function is called once after all tests have run.
    """
    logger.info("Tearing down Torua test environment...")

    if hasattr(context, 'env_manager'):
        # Stop all processes
        context.env_manager.stop_all_processes()

        # Clean up directories
        context.env_manager.cleanup_directories()

    logger.info("Test environment cleaned up")


def before_feature(context, feature):
    """
    Set up before each feature.

    This function is called before each feature file is run.
    """
    logger.info(f"Starting feature: {feature.name}")

    # Reset any feature-specific state
    context.stored_values = {}
    context.last_response = None
    context.concurrent_results = []


def after_feature(context, feature):
    """
    Tear down after each feature.

    This function is called after each feature file has run.
    """
    logger.info(f"Finished feature: {feature.name}")

    # Clean up any feature-specific resources
    if hasattr(context, 'stored_values'):
        context.stored_values.clear()


def before_scenario(context, scenario):
    """
    Set up before each scenario.

    This function is called before each scenario is run.
    """
    logger.debug(f"Starting scenario: {scenario.name}")

    # Reset scenario-specific state
    context.responses = []
    context.errors = []
    context.timing = {}


def after_scenario(context, scenario):
    """
    Tear down after each scenario.

    This function is called after each scenario has run.
    """
    logger.debug(f"Finished scenario: {scenario.name} - Status: {scenario.status}")

    # Log any errors that occurred
    if context.errors:
        for error in context.errors:
            logger.error(f"Error in scenario: {error}")

    # Clean up test data if needed
    if hasattr(context, 'test_keys'):
        # Clean up any keys created during the test
        for key in context.test_keys:
            try:
                requests.delete(
                    f"{context.test.get_coordinator_url()}/store/{key}",
                    timeout=context.test.request_timeout
                )
            except:
                pass  # Best effort cleanup
        context.test_keys = []


def before_step(context, step):
    """
    Set up before each step.

    This function is called before each step is executed.
    """
    # Record step start time for performance tracking
    context.step_start_time = time.time()


def after_step(context, step):
    """
    Tear down after each step.

    This function is called after each step has executed.
    """
    # Calculate step duration
    if hasattr(context, 'step_start_time'):
        duration = time.time() - context.step_start_time
        if duration > 1.0:  # Log slow steps
            logger.warning(f"Slow step ({duration:.2f}s): {step.name}")
