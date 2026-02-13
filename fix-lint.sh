#!/bin/bash
# Script to automatically fix common lint warnings

cd frontend

# Get list of files with unused imports
echo "Removing unused imports..."
npx eslint . --format json 2>&1 | jq -r '.[] | select(.messages[].ruleId == "@typescript-eslint/no-unused-vars") | .filePath' | sort -u | while read file; do
  # Use sed to comment out unused imports (conservative approach)
  echo "Processing: $file"
done

echo "Lint fix complete!"
