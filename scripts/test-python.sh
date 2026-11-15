#!/bin/sh
# Python AI service: pytest with line coverage, no network (LLM_PROVIDER=mock in tests).
# Run through Docker: docker compose -f compose.test.yaml run --rm python
set -eu
out=/repo/coverage/ai
mkdir -p "$out"
pip install -q --disable-pip-version-check -r requirements-dev.txt
export COVERAGE_FILE=/tmp/.coverage
python -m pytest -q -p no:cacheprovider --cov=app --cov-report=term --cov-report=html:"$out" \
  --cov-report=xml:"$out/coverage.xml" --cov-fail-under=80
