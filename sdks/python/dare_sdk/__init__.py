from .client import Diagnosis, CoreNotFoundError, diagnose
from .hook import install, uninstall

__all__ = ["Diagnosis", "CoreNotFoundError", "diagnose", "install", "uninstall"]
__version__ = "0.1.0"
