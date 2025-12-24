#!/bin/bash
###
 # @Author: Marlon.M
 # @Email: maiguangyang@163.com
 # @Date: 2025-12-24 21:55:56
### 

# Define the library path
LIB_PATH="librelay.dylib"

# Check if the library exists
if [ ! -f "$LIB_PATH" ]; then
    echo "Error: $LIB_PATH not found in the current directory."
    echo "Please run this script from the macos/ directory."
    exit 1
fi

# detailed check of the current ID
CURRENT_ID=$(otool -D "$LIB_PATH" | tail -n 1)
echo "Current LC_ID_DYLIB: $CURRENT_ID"

# The target ID using @rpath
TARGET_ID="@rpath/librelay.dylib"

# Use install_name_tool to modification the ID
echo "Updating install name to: $TARGET_ID"
install_name_tool -id "$TARGET_ID" "$LIB_PATH"

# Verify the change
NEW_ID=$(otool -D "$LIB_PATH" | tail -n 1)
echo "New LC_ID_DYLIB: $NEW_ID"

if [ "$NEW_ID" == "$TARGET_ID" ]; then
    echo "Success! The library is now ready for distribution."
else
    echo "Warning: The install name might not have been updated correctly."
fi
