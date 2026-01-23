cd $(dirname $0)
rm -Rf ../../ui
npm run build
mv build ../../ui