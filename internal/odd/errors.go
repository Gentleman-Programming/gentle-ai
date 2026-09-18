package odd

import "errors"

// Los errores centinela de este paquete permiten a las capas superiores
// (CLI, dashboard, TUI) distinguir el motivo exacto de un fallo mediante
// errors.Is, sin comparar cadenas de mensaje. [diseño §5.6]
var (
	// ErrInvalidFeatureName señala que el nombre de feature no cumple el
	// formato kebab-case o excede la longitud máxima permitida.
	ErrInvalidFeatureName = errors.New("nombre de feature no válido")
	// ErrReservedName señala que el nombre coincide con un nombre de
	// dispositivo reservado por el sistema de ficheros de Windows.
	ErrReservedName = errors.New("nombre reservado por el sistema de ficheros")
	// ErrPathEscape señala que la ruta calculada para el documento escapa de
	// la raíz del workspace tras el Join. [D-10]
	ErrPathEscape = errors.New("la ruta resultante escapa de la raíz del workspace")
	// ErrFeatureExists señala que ya existe un documento vivo con ese nombre
	// de feature.
	ErrFeatureExists = errors.New("el documento ODD ya existe")
	// ErrFeatureNotFound señala que no existe ningún documento vivo con el
	// nombre de feature solicitado.
	ErrFeatureNotFound = errors.New("documento ODD no encontrado")
	// ErrAlreadyPromoted señala que el documento vivo ya fue promovido a un
	// cambio SDD y no admite una segunda promoción. [D-08]
	ErrAlreadyPromoted = errors.New("el documento ODD ya fue promovido")
	// ErrMalformedDocument señala que el contenido leído no tiene la forma
	// mínima de un documento ODD (falta la cabecera "# ODD: <feature>").
	ErrMalformedDocument = errors.New("documento ODD malformado")
)
