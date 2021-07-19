nvidia-smi -L | grep MIG | awk '{print substr($4, 1, length($4) - 1), substr($6, 1, length($6) - 1)}' > t1
nvidia-smi | sed -e '1,/^\s*$/d' | sed '/^\s*$/q' | grep / | sed -n 'p;n' | awk '{print $5, $3}' > t2
join t2 t1 | awk '{print $2, $3}' > t3
nvidia-smi mig -lgi | grep MIG | awk '{print $6, substr($7, 1, 1)}' > t4
join t4 t3 | awk '{print "\x22"$2"\x22: \x22"$3"\x22"}' | paste -sd "," - | awk '{print "{"$0"}"}'