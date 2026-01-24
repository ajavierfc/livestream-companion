import { Link as RouterLink } from 'react-router-dom';

const SecureLink = (props) => {
  const { to, ...other } = props;
  
  // Extraemos el token de la URL actual
  const urlParams = new URLSearchParams(window.location.search);
  const token = urlParams.get('secure');

  // Si hay un token, lo añadimos a la ruta de destino
  const secureTo = token ? `${to}${to.includes('?') ? '&' : '?'}secure=${token}` : to;

  return <RouterLink {...other} to={secureTo} />;
};

export default SecureLink;