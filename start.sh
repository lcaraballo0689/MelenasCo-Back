# chmod +x melenasb
kill -9 $(cat melenasb.pid)
nohup bash -c 'exec -a meleneasb ./main' > melenas.nohup.out 2>&1 &
echo $! > melenasb.pid
PID=$(cat melenasb.pid)
echo "Iniciado melenas Backend con PID $PID"
