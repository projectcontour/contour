## JWT Authentication happens before External Authorization

Fixes a bug where when the external authorization filter and JWT authentication filter were both configured, the external authorization filter was executed _before_ the JWT authentication filter.  Now, JWT authentication happens before external authorization when they are both configured.