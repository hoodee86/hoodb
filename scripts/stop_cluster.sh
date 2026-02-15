#!/bin/bash

# 停止集群的脚本

echo "Stopping HooDB cluster..."

if [ -f "pids.txt" ]; then
    while read pid; do
        if ps -p $pid > /dev/null; then
            echo "Stopping process $pid..."
            kill $pid
        fi
    done < pids.txt
    rm pids.txt
    echo "Cluster stopped."
else
    echo "No pids.txt found. Trying to find and kill hoodb processes..."
    pkill -f "hoodb -config"
    echo "Done."
fi
