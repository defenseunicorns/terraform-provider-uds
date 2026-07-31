# Without a namespace override, use the package name as the import ID:
# <package-name>
tofu import uds_package.init init

# With a namespace override, use the namespace and package name as the import ID:
# <namespace>:<package-name>
tofu import uds_package.demo_dos_games demo:dos-games
