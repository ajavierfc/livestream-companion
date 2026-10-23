# MuxPie LiveStream Companion &middot; ![Dockerize App](https://github.com/muxpie/livestream-companion/actions/workflows/docker-image.yml/badge.svg)

## Overview

MuxPie LiveStream Companion is a robust and versatile streaming tool designed to enhance your viewing experience. It is primarily intended as a multiplexer for Plex and Emby, and can also be used as a web player for quick and straightforward streaming.

The tool is designed with simplicity and convention over configuration in mind. This means you spend less time configuring settings and more time enjoying your favorite content. MuxPie LiveStream Companion supports both Xtream Code and M3u.

## Installation

### Docker

You can run MuxPie LiveStream Companion using Docker with the following command:

```bash
docker run -p 5004:5004 -v /path/to/data:/data muxpie/livestream-companion
```

Please replace `/path/to/config` with the path to the folder containing your configuration files.

### Golang

To run the application directly from the source code, follow these steps:

1. **Clone the Repository**:
Clone the repository to your local machine by executing the following command:
```bash
git clone https://github.com/muxpie/livestream-companion.git
```

2. **Navigate to Project Directory**:
Change into the project directory with this command:
```bash
cd livestream-companion
```

3. **Build and Deploy the UI**:
Before building the Go application, you need to build and deploy the UI. Follow these steps (go 1.22.3, node v16.20.2):
- Enter the UI folder:
```bash
cd mui/livestream-companion-ui/
```
- Run the deploy script:
```bash
./deploy.sh
```
- Return to the root folder of the project:
```bash
cd ../../
```

**Install FFmpeg (if not installed)**:
The application requires FFmpeg to be installed on your machine. If you don't have it installed, please refer to the [official FFmpeg documentation](https://ffmpeg.org/download.html) for installation instructions.

**Build and Run the Application**:
Finally, you can build and run the application using these commands:
```bash
go build 
./livestream-companion
```

The application will be accessible at `http://localhost:5004`.

## Usage

Once installed, using MuxPie LiveStream Companion is a breeze. Please refer to our [User Guide](LINK_TO_USER_GUIDE) for detailed instructions.

### M3U8 playlist export

Each imported playlist can be exported as an M3U8 playlist for IPTV clients:

```text
https://<server>/playlist/<playlist-id>.m3u8
```

Add this URL as a playlist in the IPTV client. It contains channels from enabled categories only, grouped by category, with local stream URLs. No Xtream username or password is needed.

### Xtream-compatible playlist

The same playlist can also be used through an Xtream Codes-compatible API:

```text
https://<server>/xtream/<playlist-id>
```

Enter that address as the server/portal URL and provide any username and password required by the IPTV client. Credentials are accepted but not checked. Only enabled categories and their channels are returned.

Generated channel URLs follow the request protocol: local HTTP access produces HTTP URLs, while HTTPS access produces HTTPS URLs. When TLS terminates at a reverse proxy, forward the `X-Forwarded-Proto` header so exported URLs use the public protocol. Both playlist formats and their streams do not require authentication. Restrict access to a trusted network or protect the service with an authenticated reverse proxy.

For direct local access, use the service's HTTP address, such as `http://<server>:5004/playlist/<playlist-id>.m3u8` or `http://<server>:5004/xtream/<playlist-id>`.

## Disclaimer

While MuxPie LiveStream Companion makes it easy to stream content, we strongly advocate for the respect of copyright laws. It is your responsibility to ensure that any content you stream is legally permissible and done only for your own usage and that your content providers have the appropriate rights to the content they are providing.

## License

[GPLv3](./LICENSE)
