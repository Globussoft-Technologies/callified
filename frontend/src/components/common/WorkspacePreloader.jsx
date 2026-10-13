import logo from '../../assets/tg_image_3608761279.png';
import './WorkspacePreloader.css';

export default function WorkspacePreloader({ message = 'Opening your workspace...' }) {
  return <div className="workspace-preloader" role="status" aria-live="polite" aria-busy="true">
    <div className="workspace-preloader-content">
      <img src={logo} alt="" />
      <h1>Callified AI</h1>
      <p>{message}</p>
      <div className="workspace-preloader-bars" aria-hidden="true">
        {Array.from({ length: 9 }, (_, index) => <i key={index} style={{ animationDelay: `${index * -0.12}s` }} />)}
      </div>
    </div>
  </div>;
}
