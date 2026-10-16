package httpapi

import "github.com/go-chi/chi/v5"

// mounts lists the feature modules registered under the authenticated API group.
func (s *Server) mounts() []func(chi.Router) {
	return []func(chi.Router){
		s.mountCompanies, s.mountContacts, s.mountSuppliers, s.mountDeals, s.mountActivity, s.mountSearch,
		s.mountProducts, s.mountImports, s.mountCaptures, s.mountAnalysis,
	}
}
