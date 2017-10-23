#include <a_samp>
#include "filemanager"

main() {
	new File:f = fopen("dir/test.txt");
	fwrite(f, "hello world\n");
	fclose(f);

	new
		dir:dirhandle = dir_open("scriptfiles"),
		item[64],
		type;

	if (!dirhandle) {
		print("UNKNOWN ERROR: Failed to read server directory");
	}

	print("directory scan:");
	while (dir_list(dirhandle, item, type)) {
		print(item);
	}
}
