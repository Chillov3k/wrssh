use std::future::Future;
use std::pin::Pin;
use std::task::{Context, Poll};

#[derive(Debug)]
pub struct JoinError;

impl std::fmt::Display for JoinError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "JoinError")
    }
}

impl std::error::Error for JoinError {}

pub struct JoinHandle<T>
where
    T: Send,
{
    handle: std::sync::Arc<tokio::sync::Mutex<Option<tokio::sync::oneshot::Receiver<T>>>>,
}

impl<T> Clone for JoinHandle<T>
where
    T: Send,
{
    fn clone(&self) -> Self {
        Self {
            handle: self.handle.clone(),
        }
    }
}

#[cfg(target_arch = "wasm32")]
macro_rules! spawn_impl {
    ($fn:expr) => {
        wasm_bindgen_futures::spawn_local($fn)
    };
}

#[cfg(not(target_arch = "wasm32"))]
macro_rules! spawn_impl {
    ($fn:expr) => {
        tokio::spawn($fn)
    };
}

pub fn spawn<F, T>(future: F) -> JoinHandle<T>
where
    F: Future<Output = T> + 'static + Send,
    T: Send + 'static,
{
    let (sender, receiver) = tokio::sync::oneshot::channel();
    spawn_impl!(async {
        let result = future.await;
        let _ = sender.send(result);
    });
    JoinHandle {
        handle: std::sync::Arc::new(tokio::sync::Mutex::new(Some(receiver))),
    }
}

impl<T> Future for JoinHandle<T>
where
    T: Send,
{
    type Output = Result<T, JoinError>;

    fn poll(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Self::Output> {
        let inner = self.handle.clone();
        let mut guard = match inner.try_lock() {
            Ok(g) => g,
            Err(_) => return Poll::Pending,
        };
        match guard.as_mut() {
            Some(rx) => match Pin::new(rx).poll(cx) {
                Poll::Ready(Ok(val)) => {
                    *guard = None;
                    Poll::Ready(Ok(val))
                }
                Poll::Ready(Err(_)) => {
                    *guard = None;
                    Poll::Ready(Err(JoinError))
                }
                Poll::Pending => Poll::Pending,
            },
            None => Poll::Ready(Err(JoinError)),
        }
    }
}
