// Axiom Enterprise Dashboard — Client Logic
document.addEventListener('DOMContentLoaded', () => {
  let workspaceData = null;
  let incrementsData = [];
  let currentFilter = 'all';

  // Elementos DOM
  const wsNameEl = document.getElementById('ws-name');
  const wsTopologyEl = document.getElementById('ws-topology');
  const incrementsContainer = document.getElementById('increments-container');
  const roleChangeSelect = document.getElementById('role-change-select');
  const rolesContainer = document.getElementById('roles-container');
  const barrierBanner = document.getElementById('barrier-result-card');
  const barrierIcon = document.getElementById('barrier-icon');
  const barrierTitle = document.getElementById('barrier-title');
  const barrierDesc = document.getElementById('barrier-desc');
  const handoffChangeSelect = document.getElementById('handoff-change-select');
  const handoffDisplay = document.getElementById('handoff-display');
  const handoffEmpty = document.getElementById('handoff-empty');
  const hoFromPhase = document.getElementById('ho-from-phase');
  const hoToPhase = document.getElementById('ho-to-phase');
  const hoRoles = document.getElementById('ho-roles');
  const hoStatus = document.getElementById('ho-status');
  const hoTime = document.getElementById('ho-time');
  const hoSectionsContainer = document.getElementById('ho-sections-container');
  const skillsContainer = document.getElementById('skills-container');

  // Modal
  const modal = document.getElementById('inc-modal');
  const modalTitle = document.getElementById('modal-title');
  const modalContent = document.getElementById('modal-content');
  const btnModalClose = document.getElementById('btn-modal-close');

  // Navegación por Pestañas
  document.querySelectorAll('.nav-tab').forEach(tab => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.nav-tab').forEach(t => t.classList.remove('active'));
      document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
      tab.classList.add('active');
      const panelId = tab.getAttribute('data-tab');
      const targetPanel = document.getElementById(panelId);
      if (targetPanel) targetPanel.classList.add('active');
    });
  });

  // Filtro de Incrementos
  document.querySelectorAll('.filter-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      currentFilter = btn.getAttribute('data-filter');
      renderIncrements();
    });
  });

  // Botón Actualizar
  document.getElementById('btn-refresh').addEventListener('click', () => {
    loadAllData();
  });

  // Cerrar Modal
  btnModalClose.addEventListener('click', () => modal.classList.add('hidden'));
  modal.addEventListener('click', (e) => {
    if (e.target === modal) modal.classList.add('hidden');
  });

  // Selectores de Cambios
  roleChangeSelect.addEventListener('change', () => {
    const val = roleChangeSelect.value;
    if (val) loadRolesForChange(val);
    else {
      rolesContainer.innerHTML = '<p class="empty-state">Selecciona un cambio arriba.</p>';
      barrierBanner.classList.add('hidden');
    }
  });

  handoffChangeSelect.addEventListener('change', () => {
    const val = handoffChangeSelect.value;
    if (val) loadHandoffForChange(val);
    else {
      handoffDisplay.classList.add('hidden');
      handoffEmpty.classList.remove('hidden');
    }
  });

  // Carga inicial
  loadAllData();

  async function loadAllData() {
    await Promise.all([
      loadWorkspace(),
      loadIncrements(),
      loadSkills()
    ]);
  }

  // 1. Cargar Workspace
  async function loadWorkspace() {
    try {
      const res = await fetch('/api/workspace');
      if (!res.ok) throw new Error('Fallo al obtener workspace');
      workspaceData = await res.json();

      wsNameEl.textContent = workspaceData.name || 'Workspace';
      wsTopologyEl.textContent = workspaceData.topology || 'monorepo';

      // Actualizar Tab de Topología
      document.getElementById('top-ws-name').textContent = workspaceData.name;
      document.getElementById('top-ws-topology').textContent = workspaceData.topology;
      document.getElementById('top-ws-specs').textContent = workspaceData.specs_repository || 'openspec/';
      const healthEl = document.getElementById('top-ws-health');
      if (workspaceData.compliant) {
        healthEl.textContent = 'CONFORME';
        healthEl.className = 'stat-value text-success';
      } else {
        healthEl.textContent = 'NO CONFORME';
        healthEl.className = 'stat-value text-danger';
      }

      // Tabla de Roles en Topología
      const tbody = document.getElementById('topology-roles-tbody');
      tbody.innerHTML = '';
      if (workspaceData.roles) {
        Object.entries(workspaceData.roles).forEach(([id, r]) => {
          const tr = document.createElement('tr');
          const repos = (r.repositories || []).join(', ') || '.';
          const tech = (r.tech || []).join(', ') || 'N/A';
          tr.innerHTML = `
            <td><code>${id}</code></td>
            <td><strong>${r.name || id}</strong></td>
            <td><span class="badge badge-${r.gate_policy}">${r.gate_policy || 'blocking'}</span></td>
            <td>${tech}</td>
            <td><code>${repos}</code></td>
          `;
          tbody.appendChild(tr);
        });
      }
    } catch (err) {
      console.error(err);
      wsNameEl.textContent = 'Error al conectar';
    }
  }

  // 2. Cargar Incrementos
  async function loadIncrements() {
    try {
      const res = await fetch('/api/increments');
      if (!res.ok) throw new Error('Fallo al obtener incrementos');
      incrementsData = await res.json() || [];

      // Llenar selectores
      updateChangeSelects(incrementsData);
      renderIncrements();
    } catch (err) {
      console.error(err);
      incrementsContainer.innerHTML = '<div class="empty-state text-danger">Error al cargar incrementos.</div>';
    }
  }

  function updateChangeSelects(items) {
    const prevRoleVal = roleChangeSelect.value;
    const prevHoVal = handoffChangeSelect.value;

    roleChangeSelect.innerHTML = '<option value="">Seleccionar cambio...</option>';
    handoffChangeSelect.innerHTML = '<option value="">Seleccionar cambio...</option>';

    items.forEach(inc => {
      const opt1 = document.createElement('option');
      opt1.value = inc.name;
      opt1.textContent = `${inc.name} (${inc.type === 'active' ? 'Activo' : 'Archivado'})`;
      roleChangeSelect.appendChild(opt1);

      const opt2 = document.createElement('option');
      opt2.value = inc.name;
      opt2.textContent = `${inc.name} (${inc.type === 'active' ? 'Activo' : 'Archivado'})`;
      handoffChangeSelect.appendChild(opt2);
    });

    if (prevRoleVal) roleChangeSelect.value = prevRoleVal;
    if (prevHoVal) handoffChangeSelect.value = prevHoVal;
  }

  function renderIncrements() {
    incrementsContainer.innerHTML = '';
    const filtered = incrementsData.filter(i => {
      if (currentFilter === 'active') return i.type === 'active';
      if (currentFilter === 'archived') return i.type === 'archived';
      return true;
    });

    if (filtered.length === 0) {
      incrementsContainer.innerHTML = '<div class="empty-state">No se encontraron incrementos para este filtro.</div>';
      return;
    }

    filtered.forEach(inc => {
      const card = document.createElement('div');
      card.className = `inc-card ${inc.type}`;
      const isArchived = inc.type === 'archived';

      card.innerHTML = `
        <div>
          <div class="card-header">
            <div class="card-title">${inc.name}</div>
            <div class="card-badges">
              <span class="badge badge-${inc.type}">${isArchived ? 'Archivado' : 'Activo'}</span>
              <span class="badge badge-phase">${inc.phase}</span>
            </div>
          </div>
          <div class="progress-container">
            <div class="progress-info">
              <span>Tareas: ${inc.tasks_completed}/${inc.tasks_total}</span>
              <span>${inc.progress_pct}%</span>
            </div>
            <div class="progress-track">
              <div class="progress-bar" style="width: ${inc.progress_pct}%"></div>
            </div>
          </div>
        </div>
        <div class="card-footer">
          <span class="card-date">${inc.date ? 'Fecha: ' + inc.date : 'En progreso'}</span>
          <button class="btn btn-secondary btn-detail" data-name="${inc.name}">Ver Detalle</button>
        </div>
      `;

      card.querySelector('.btn-detail').addEventListener('click', () => {
        openIncrementModal(inc.name);
      });

      incrementsContainer.appendChild(card);
    });
  }

  async function openIncrementModal(name) {
    modalTitle.textContent = `Incremento: ${name}`;
    modalContent.innerHTML = '<div class="loading-state">Cargando artefactos...</div>';
    modal.classList.remove('hidden');

    try {
      const res = await fetch(`/api/increments/${name}`);
      if (!res.ok) throw new Error('No se pudo obtener el detalle');
      const data = await res.json();

      let html = `
        <div style="display: flex; gap: 0.5rem; margin-bottom: 1rem;">
          <span class="badge badge-${data.summary.type}">${data.summary.type}</span>
          <span class="badge badge-phase">Fase: ${data.summary.phase}</span>
          <span class="badge badge-pass">Tareas: ${data.summary.tasks_completed}/${data.summary.tasks_total} (${data.summary.progress_pct}%)</span>
        </div>
        <h4>Artefactos Detectados:</h4>
        <ul style="margin-left: 1.5rem; margin-bottom: 1.25rem;">
          <li>Propuesta (proposal.md): <strong>${data.has_proposal ? '✓ Presente' : '✗ Ausente'}</strong></li>
          <li>Especificación (spec.md): <strong>${data.has_spec ? '✓ Presente' : '✗ Ausente'}</strong></li>
          <li>Diseño Técnico (design.md): <strong>${data.has_design ? '✓ Presente' : '✗ Ausente'}</strong></li>
          <li>Plan de Tareas (tasks.md): <strong>${data.has_tasks ? '✓ Presente' : '✗ Ausente'}</strong></li>
          <li>Informe Verificación (verify-report.md): <strong>${data.has_verify ? '✓ Presente' : '✗ Ausente'}</strong></li>
          <li>Informe Archivado (archive-report.md): <strong>${data.has_archive ? '✓ Presente' : '✗ Ausente'}</strong></li>
        </ul>
      `;

      if (data.proposal) {
        html += `<h4>Vista Previa de Propuesta:</h4><pre>${escapeHtml(data.proposal.substring(0, 500))}...</pre>`;
      }
      modalContent.innerHTML = html;
    } catch (err) {
      modalContent.innerHTML = `<div class="text-danger">Error: ${err.message}</div>`;
    }
  }

  // 3. Cargar Roles y Barrera para un cambio
  async function loadRolesForChange(changeName) {
    rolesContainer.innerHTML = '<div class="loading-state">Evaluando roles y barrera...</div>';
    try {
      const res = await fetch(`/api/roles?change=${encodeURIComponent(changeName)}`);
      if (!res.ok) throw new Error('Fallo al evaluar roles');
      const barrier = await res.json();

      // Banner de la Barrera
      barrierBanner.classList.remove('hidden');
      if (barrier.satisfied) {
        barrierBanner.className = 'barrier-banner satisfied';
        barrierIcon.textContent = '✓';
        barrierTitle.textContent = 'BARRIER SATISFIED (Compuerta Superada)';
        barrierDesc.textContent = 'Todos los roles obligatorios (blocking) han verificado exitosamente sus tareas.';
      } else {
        barrierBanner.className = 'barrier-banner blocked';
        barrierIcon.textContent = '✕';
        barrierTitle.textContent = 'BARRIER BLOCKED (Compuerta Bloqueada)';
        barrierDesc.textContent = (barrier.blockers && barrier.blockers.length > 0)
          ? barrier.blockers.join(' • ')
          : 'Existen roles obligatorios con tareas pendientes o verificación fallida.';
      }

      // Renderizar tarjetas de cada rol
      rolesContainer.innerHTML = '';
      if (!barrier.roles || Object.keys(barrier.roles).length === 0) {
        rolesContainer.innerHTML = '<p class="empty-state">No se detectaron roles específicos (modo mono-rol).</p>';
        return;
      }

      Object.entries(barrier.roles).forEach(([roleId, status]) => {
        const rCard = document.createElement('div');
        rCard.className = 'role-card';

        const isBlocking = status.role.gate_policy === 'blocking';
        const isVerified = status.verify_verdict === 'pass';

        rCard.innerHTML = `
          <div>
            <div class="card-header">
              <div class="card-title">${status.role.name || roleId}</div>
              <div class="card-badges">
                <span class="badge badge-${status.role.gate_policy}">${status.role.gate_policy}</span>
                <span class="badge ${isVerified ? 'badge-pass' : 'badge-fail'}">Verif: ${status.verify_verdict || 'pendiente'}</span>
              </div>
            </div>
            <div class="progress-container">
              <div class="progress-info">
                <span>Tareas: ${status.progress.completed}/${status.progress.total}</span>
                <span>${status.progress.percentage}%</span>
              </div>
              <div class="progress-track">
                <div class="progress-bar" style="width: ${status.progress.percentage}%"></div>
              </div>
            </div>
            <p style="font-size: 0.8rem; color: var(--text-muted); margin-top: 0.5rem;">
              Repositorios: <code>${(status.role.repositories || []).join(', ') || '.'}</code>
            </p>
          </div>
        `;
        rolesContainer.appendChild(rCard);
      });
    } catch (err) {
      console.error(err);
      rolesContainer.innerHTML = `<div class="empty-state text-danger">Error: ${err.message}</div>`;
      barrierBanner.classList.add('hidden');
    }
  }

  // 4. Cargar Handoff para un cambio
  async function loadHandoffForChange(changeName) {
    try {
      const res = await fetch(`/api/handoffs?change=${encodeURIComponent(changeName)}`);
      if (!res.ok) {
        handoffDisplay.classList.add('hidden');
        handoffEmpty.classList.remove('hidden');
        handoffEmpty.textContent = `No se encontró archivo handoff.md para "${changeName}".`;
        return;
      }

      const ho = await res.json();
      handoffEmpty.classList.add('hidden');
      handoffDisplay.classList.remove('hidden');

      hoFromPhase.textContent = ho.metadata.from_phase || 'origen';
      hoToPhase.textContent = ho.metadata.to_phase || 'destino';
      hoRoles.textContent = `${ho.metadata.from_role} ➔ ${ho.metadata.to_role}`;
      hoStatus.textContent = ho.metadata.status || 'ready';
      hoStatus.className = `badge ${ho.metadata.status === 'ready' ? 'badge-pass' : 'badge-fail'}`;
      hoTime.textContent = ho.metadata.timestamp ? new Date(ho.metadata.timestamp).toLocaleString() : '--';

      hoSectionsContainer.innerHTML = '';
      const sections = [
        { num: '1', title: 'Resumen Ejecutivo', text: ho.sections.executive_summary },
        { num: '2', title: 'Artefactos Modificados y Creados', text: ho.sections.artifacts },
        { num: '3', title: 'Decisiones Técnicas y Acuerdos', text: ho.sections.decisions },
        { num: '4', title: 'Riesgos, Bloqueos y Preguntas Abiertas', text: ho.sections.risks_and_blockers },
        { num: '5', title: 'Instrucciones Directas para el Siguiente Rol', text: ho.sections.next_instructions }
      ];

      sections.forEach(s => {
        const box = document.createElement('div');
        box.className = 'section-box';
        box.innerHTML = `
          <h4>${s.num}. ${s.title}</h4>
          <div class="section-content">${escapeHtml(s.text || 'Sin contenido.')}</div>
        `;
        hoSectionsContainer.appendChild(box);
      });
    } catch (err) {
      console.error(err);
      handoffDisplay.classList.add('hidden');
      handoffEmpty.classList.remove('hidden');
      handoffEmpty.textContent = 'Error cargando handoff.';
    }
  }

  // 5. Cargar Skills
  async function loadSkills() {
    try {
      const res = await fetch('/api/skills');
      if (!res.ok) throw new Error('Fallo al obtener skills');
      const skills = await res.json() || [];

      skillsContainer.innerHTML = '';
      if (skills.length === 0) {
        skillsContainer.innerHTML = '<p class="empty-state">No se encontraron skills locales.</p>';
        return;
      }

      skills.forEach(sk => {
        const card = document.createElement('div');
        card.className = 'skill-card';
        card.innerHTML = `
          <div>
            <div class="card-header">
              <div class="card-title">${sk.name}</div>
              <span class="badge badge-phase">Skill</span>
            </div>
            <p style="font-size: 0.85rem; color: var(--text-secondary); margin-bottom: 0.85rem;">
              ${sk.description || 'Sin descripción'}
            </p>
          </div>
          <div class="card-footer">
            <span style="font-size: 0.72rem; color: var(--text-muted);"><code>${sk.path}</code></span>
          </div>
        `;
        skillsContainer.appendChild(card);
      });
    } catch (err) {
      console.error(err);
      skillsContainer.innerHTML = '<p class="empty-state text-danger">Error cargando catálogo de skills.</p>';
    }
  }

  function escapeHtml(str) {
    if (!str) return '';
    return str
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#039;");
  }
});
