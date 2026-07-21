import React from 'react';

/**
 * AccessibleOverlay is a modal backdrop that is keyboard-operable.
 * SonarQube S1082 requires visible, non-interactive elements with click
 * handlers to also support keyboard interaction. This component adds
 * role="button", tabIndex={0} and an Enter/Space key handler that triggers
 * the close action.
 */
export default function AccessibleOverlay({ children, onClose, className = 'modal-overlay', ...rest }) {
  const handleKeyDown = (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      e.stopPropagation();
      onClose(e);
    }
  };

  return (
    <div
      className={className}
      role="button"
      tabIndex={0}
      onClick={onClose}
      onKeyDown={handleKeyDown}
      {...rest}
    >
      {children}
    </div>
  );
}
