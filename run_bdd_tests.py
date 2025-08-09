#!/usr/bin/env python3
"""
BDD test runner for Torua distributed storage system.

This script runs the Behave tests with appropriate configuration and reporting.
It can be used for local development, CI/CD pipelines, and test automation.
"""

import os
import sys
import argparse
import subprocess
import json
import time
from pathlib import Path
from typing import List, Optional, Dict, Any

# Add project root to path
project_root = Path(__file__).parent
sys.path.insert(0, str(project_root))


class BDDTestRunner:
    """Runner for BDD tests using Behave."""

    def __init__(self, args: argparse.Namespace):
        """Initialize the test runner with command-line arguments."""
        self.args = args
        self.project_root = project_root
        self.features_dir = self.project_root / "features"
        self.results_dir = self.project_root / "test-results"
        self.coverage_dir = self.project_root / "coverage"

        # Ensure directories exist
        self.results_dir.mkdir(parents=True, exist_ok=True)
        if args.coverage:
            self.coverage_dir.mkdir(parents=True, exist_ok=True)

    def build_services(self) -> bool:
        """Build the coordinator and node services."""
        print("Building services...")

        # Build coordinator
        result = subprocess.run(
            ["make", "build-coordinator"],
            cwd=self.project_root,
            capture_output=True,
            text=True
        )
        if result.returncode != 0:
            print(f"Failed to build coordinator: {result.stderr}")
            return False

        # Build node
        result = subprocess.run(
            ["make", "build-node"],
            cwd=self.project_root,
            capture_output=True,
            text=True
        )
        if result.returncode != 0:
            print(f"Failed to build node: {result.stderr}")
            return False

        print("Services built successfully")
        return True

    def install_dependencies(self) -> bool:
        """Install Python dependencies for testing."""
        print("Installing test dependencies...")

        requirements_file = self.project_root / "requirements-test.txt"
        if not requirements_file.exists():
            print(f"Requirements file not found: {requirements_file}")
            return False

        result = subprocess.run(
            [sys.executable, "-m", "pip", "install", "-r", str(requirements_file)],
            capture_output=True,
            text=True
        )

        if result.returncode != 0:
            print(f"Failed to install dependencies: {result.stderr}")
            return False

        print("Dependencies installed successfully")
        return True

    def build_behave_command(self) -> List[str]:
        """Build the behave command with appropriate options."""
        cmd = ["behave"]

        # Add feature path or specific feature
        if self.args.feature:
            cmd.append(f"features/{self.args.feature}")
        else:
            cmd.append("features/")

        # Add tags
        if self.args.tags:
            for tag in self.args.tags:
                cmd.extend(["-t", tag])

        # Add format options
        if self.args.format:
            cmd.extend(["--format", self.args.format])
        else:
            # Default formats
            cmd.extend(["--format", "pretty"])
            if not self.args.no_junit:
                cmd.extend(["--junit"])
                cmd.extend(["--junit-directory", str(self.results_dir)])

        # Add verbosity
        if self.args.verbose:
            cmd.append("-v")

        # Add dry run
        if self.args.dry_run:
            cmd.append("--dry-run")

        # Add stop on first failure
        if self.args.fail_fast:
            cmd.append("--stop")

        # Add specific scenario
        if self.args.scenario:
            cmd.extend(["--name", self.args.scenario])

        # Add no-capture for debugging
        if self.args.debug:
            cmd.append("--no-capture")
            cmd.append("--no-capture-stderr")
            cmd.append("--no-logcapture")

        # Add user data
        user_data = {}
        if self.args.coordinator_port:
            user_data["coordinator_port"] = str(self.args.coordinator_port)
        if self.args.node1_port:
            user_data["node1_port"] = str(self.args.node1_port)
        if self.args.node2_port:
            user_data["node2_port"] = str(self.args.node2_port)
        if self.args.startup_timeout:
            user_data["startup_timeout"] = str(self.args.startup_timeout)
        if self.args.no_cleanup:
            user_data["cleanup_on_exit"] = "false"

        for key, value in user_data.items():
            cmd.extend(["-D", f"{key}={value}"])

        return cmd

    def run_tests(self) -> int:
        """Run the BDD tests."""
        # Build services if requested
        if not self.args.no_build:
            if not self.build_services():
                return 1

        # Install dependencies if requested
        if self.args.install_deps:
            if not self.install_dependencies():
                return 1

        # Build behave command
        cmd = self.build_behave_command()

        print(f"Running command: {' '.join(cmd)}")
        print("-" * 60)

        # Run behave
        if self.args.coverage:
            # Run with coverage
            coverage_cmd = [
                sys.executable, "-m", "coverage", "run",
                "--source=features",
                "--omit=*/test_*.py,*/tests/*",
                "-m", "behave"
            ] + cmd[1:]  # Skip 'behave' from original command

            result = subprocess.run(
                coverage_cmd,
                cwd=self.project_root,
                env={**os.environ, "PYTHONPATH": str(self.project_root)}
            )

            if result.returncode == 0:
                # Generate coverage report
                subprocess.run(
                    [sys.executable, "-m", "coverage", "report"],
                    cwd=self.project_root
                )
                subprocess.run(
                    [sys.executable, "-m", "coverage", "html", "--directory", str(self.coverage_dir)],
                    cwd=self.project_root
                )
                print(f"\nCoverage report generated in {self.coverage_dir}")
        else:
            # Run without coverage
            result = subprocess.run(
                cmd,
                cwd=self.project_root,
                env={**os.environ, "PYTHONPATH": str(self.project_root)}
            )

        return result.returncode

    def list_features(self):
        """List available feature files."""
        print("Available features:")
        print("-" * 40)

        for feature_file in sorted(self.features_dir.glob("*.feature")):
            print(f"  {feature_file.stem}")

            # Read first few lines to get feature description
            with open(feature_file, 'r') as f:
                for line in f:
                    line = line.strip()
                    if line.startswith("Feature:"):
                        print(f"    {line}")
                        break

        print()
        print("Run a specific feature with: --feature <name>.feature")

    def list_scenarios(self):
        """List all scenarios across all features."""
        print("Available scenarios:")
        print("-" * 40)

        for feature_file in sorted(self.features_dir.glob("*.feature")):
            print(f"\n{feature_file.stem}:")

            with open(feature_file, 'r') as f:
                for line in f:
                    line = line.strip()
                    if line.startswith("Scenario:") or line.startswith("Scenario Outline:"):
                        # Extract scenario name
                        scenario_name = line.split(":", 1)[1].strip()
                        print(f"  - {scenario_name}")

        print()
        print("Run a specific scenario with: --scenario \"<scenario name>\"")


def main():
    """Main entry point for the test runner."""
    parser = argparse.ArgumentParser(
        description="Run BDD tests for Torua distributed storage system",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Run all tests
  python run_bdd_tests.py

  # Run a specific feature
  python run_bdd_tests.py --feature distributed-storage.feature

  # Run tests with specific tags
  python run_bdd_tests.py -t @integration -t ~@slow

  # Run with coverage
  python run_bdd_tests.py --coverage

  # Debug mode with verbose output
  python run_bdd_tests.py --debug --verbose

  # List available features
  python run_bdd_tests.py --list-features
        """
    )

    # Test selection options
    parser.add_argument(
        "--feature", "-f",
        help="Run a specific feature file (e.g., distributed-storage.feature)"
    )
    parser.add_argument(
        "--scenario", "-s",
        help="Run scenarios matching this name"
    )
    parser.add_argument(
        "--tags", "-t",
        action="append",
        help="Run scenarios with specific tags (can be used multiple times)"
    )

    # Output options
    parser.add_argument(
        "--format",
        choices=["pretty", "json", "plain", "progress"],
        help="Output format (default: pretty)"
    )
    parser.add_argument(
        "--no-junit",
        action="store_true",
        help="Don't generate JUnit XML reports"
    )
    parser.add_argument(
        "--coverage",
        action="store_true",
        help="Run with code coverage analysis"
    )

    # Execution options
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Verify steps are defined without executing them"
    )
    parser.add_argument(
        "--fail-fast",
        action="store_true",
        help="Stop on first failure"
    )
    parser.add_argument(
        "--debug",
        action="store_true",
        help="Enable debug mode (no output capture)"
    )
    parser.add_argument(
        "--verbose", "-v",
        action="store_true",
        help="Verbose output"
    )

    # Environment options
    parser.add_argument(
        "--coordinator-port",
        type=int,
        default=8080,
        help="Coordinator port (default: 8080)"
    )
    parser.add_argument(
        "--node1-port",
        type=int,
        default=8081,
        help="Node 1 port (default: 8081)"
    )
    parser.add_argument(
        "--node2-port",
        type=int,
        default=8082,
        help="Node 2 port (default: 8082)"
    )
    parser.add_argument(
        "--startup-timeout",
        type=int,
        default=10,
        help="Service startup timeout in seconds (default: 10)"
    )
    parser.add_argument(
        "--no-cleanup",
        action="store_true",
        help="Don't clean up test data after tests"
    )

    # Build options
    parser.add_argument(
        "--no-build",
        action="store_true",
        help="Skip building services before tests"
    )
    parser.add_argument(
        "--install-deps",
        action="store_true",
        help="Install Python dependencies before running tests"
    )

    # Information options
    parser.add_argument(
        "--list-features",
        action="store_true",
        help="List available feature files and exit"
    )
    parser.add_argument(
        "--list-scenarios",
        action="store_true",
        help="List all scenarios and exit"
    )

    args = parser.parse_args()

    # Create runner
    runner = BDDTestRunner(args)

    # Handle information requests
    if args.list_features:
        runner.list_features()
        return 0

    if args.list_scenarios:
        runner.list_scenarios()
        return 0

    # Run tests
    return runner.run_tests()


if __name__ == "__main__":
    sys.exit(main())
