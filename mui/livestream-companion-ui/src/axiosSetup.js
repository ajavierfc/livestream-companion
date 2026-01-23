import axiosLib from 'axios';

const axios = axiosLib.create({
  baseURL: process.env.REACT_APP_API_BASE_URL || window.location.origin,
});

axios.interceptors.request.use(
  (config) => {
    const urlParams = new URLSearchParams(window.location.search);
    const token = urlParams.get('secure');

    if (token) {
      config.params = {
        ...config.params,
        secure: token,
      };
    }

    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

export default axios;