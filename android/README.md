# Hiveloop for Android

A minimal app: it opens your Hiveloop address full screen (WebView), without browser bars.
It handles file uploads, document downloads, the Back button and external links (opened in the browser).

## Building with GitHub Actions

`.github/workflows/android.yml` builds and signs the APK and publishes it as the `android-latest`
release. Run it by hand from *Actions → Android app → Run workflow* and enter the address the app
should open (or set it once as the repository variable `HIVELOOP_URL`).

### Signing key (once)

Android only installs updates signed with the **same key**, so create one and keep a backup of it.
It never goes into the repository:

```sh
mkdir -p ~/.config/hiveloop/android && cd ~/.config/hiveloop/android
openssl rand -base64 24 > password.txt
keytool -genkeypair -keystore hiveloop-release.p12 -storetype pkcs12 -alias hiveloop \
  -keyalg RSA -keysize 3072 -validity 10000 -dname "CN=Hiveloop" \
  -storepass "$(cat password.txt)" -keypass "$(cat password.txt)"
```

Then store it as repository secrets:

```sh
base64 -w0 hiveloop-release.p12 | gh secret set ANDROID_KEYSTORE_B64
gh secret set ANDROID_KEYSTORE_PASSWORD < password.txt
```

### Offering the APK from your server

Hiveloop serves `data/downloads/hiveloop.apk` at `/download/hiveloop.apk`, and the sign-up and
profile pages show a download button when the file is there. After each build:

```sh
mkdir -p data/downloads
gh release download android-latest --pattern hiveloop.apk --dir data/downloads --clobber
```

## iPhone / iPad

No app is needed: Hiveloop offers a **Web Clip** profile (`/download/hiveloop.mobileconfig`) that adds
its icon to the Home Screen and opens it full screen, or use "Add to Home Screen" in Safari.
