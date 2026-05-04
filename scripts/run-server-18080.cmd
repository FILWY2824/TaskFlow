@echo off
cd /d "%~dp0..\server"
set TASKFLOW_LISTEN=127.0.0.1:18080
set TASKFLOW_DB_PATH=data\taskflow-18080.db
taskflow-server.exe -config config.toml -env ..\.env
